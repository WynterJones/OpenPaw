package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
)

func searchFilesReq(t *testing.T, h *WorkspacesHandler, query string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Get("/workspaces/{id}/search", h.SearchFiles)
	req := httptest.NewRequest(http.MethodGet,
		"/workspaces/"+DefaultWorkspaceID+"/search?q="+url.QueryEscape(query), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestSearchFiles_StaysInWorkspaceAndAttachedDirectories(t *testing.T) {
	h, files := newTestWorkspacesHandler(t)
	if err := os.MkdirAll(filepath.Join(files, "projects", "openpaw"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(files, "projects", "openpaw", "roadmap.md"), []byte("plan"), 0o644)
	for _, ignoredPath := range []string{
		"node_modules/hidden/roadmap.md",
		".claude/worktrees/branch/roadmap.md",
		"worktrees/branch/roadmap.md",
	} {
		fullPath := filepath.Join(files, ignoredPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(fullPath, []byte("noise"), 0o644)
	}

	attached := t.TempDir()
	_ = os.WriteFile(filepath.Join(attached, "favorite-sites.md"), []byte("links"), 0o644)
	dirID := uuid.NewString()
	if _, err := h.db.Exec(
		"INSERT INTO workspace_directories (id, workspace_id, path, label) VALUES (?, ?, ?, ?)",
		dirID, DefaultWorkspaceID, attached, "Reference",
	); err != nil {
		t.Fatal(err)
	}

	rec := searchFilesReq(t, h, "roadmap")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var results []workspaceSearchResult
	decodeTestJSON(t, rec, &results)
	if len(results) != 1 || results[0].Path != "projects/openpaw/roadmap.md" {
		t.Fatalf("workspace results = %#v", results)
	}
	if !strings.HasPrefix(results[0].AbsolutePath, files) {
		t.Fatalf("result escaped workspace: %s", results[0].AbsolutePath)
	}

	rec = searchFilesReq(t, h, "favorite")
	decodeTestJSON(t, rec, &results)
	if len(results) != 1 || results[0].DirID != dirID || results[0].Source != "Reference" {
		t.Fatalf("attached results = %#v", results)
	}
}

func TestSearchFiles_FuzzyMatchesAndRanksNames(t *testing.T) {
	h, files := newTestWorkspacesHandler(t)
	for _, path := range []string{
		"notes/openpaw-report.md",
		"notes/old-product-roadmap.md",
		"reports/openpaw-release-draft.md",
	} {
		fullPath := filepath.Join(files, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	rec := searchFilesReq(t, h, "oprp")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var results []workspaceSearchResult
	decodeTestJSON(t, rec, &results)
	if len(results) == 0 {
		t.Fatalf("fuzzy results = %#v, want an ordered-character match", results)
	}
	if results[0].Name != "openpaw-report.md" {
		t.Fatalf("first fuzzy result = %q, want closest filename match", results[0].Name)
	}

	rec = searchFilesReq(t, h, "reports draft")
	decodeTestJSON(t, rec, &results)
	if len(results) != 1 || results[0].Name != "openpaw-release-draft.md" {
		t.Fatalf("multi-term fuzzy results = %#v", results)
	}
}

func TestSearchFiles_IncludesSelectableFoldersAndAttachedRoots(t *testing.T) {
	h, files := newTestWorkspacesHandler(t)
	nested := filepath.Join(files, "projects", "client-portal")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	attachedParent := t.TempDir()
	attached := filepath.Join(attachedParent, "Design Assets")
	if err := os.MkdirAll(filepath.Join(attached, "logos"), 0o755); err != nil {
		t.Fatal(err)
	}
	dirID := uuid.NewString()
	if _, err := h.db.Exec(
		"INSERT INTO workspace_directories (id, workspace_id, path, label) VALUES (?, ?, ?, ?)",
		dirID, DefaultWorkspaceID, attached, "Design Assets",
	); err != nil {
		t.Fatal(err)
	}

	rec := searchFilesReq(t, h, "client portal")
	if rec.Code != http.StatusOK {
		t.Fatalf("nested folder status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var results []workspaceSearchResult
	decodeTestJSON(t, rec, &results)
	if len(results) == 0 || !results[0].IsDir || results[0].Path != "projects/client-portal" {
		t.Fatalf("nested folder results = %#v", results)
	}

	rec = searchFilesReq(t, h, "design assets")
	if rec.Code != http.StatusOK {
		t.Fatalf("attached root status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	decodeTestJSON(t, rec, &results)
	if len(results) == 0 {
		t.Fatal("attached directory root was not searchable")
	}
	root := results[0]
	if !root.IsDir || root.DirID != dirID || root.Path != "" || root.AbsolutePath != attached {
		t.Fatalf("attached root = %#v", root)
	}
}

func newTestWorkspacesHandler(t *testing.T) (*WorkspacesHandler, string) {
	t.Helper()
	dataDir := t.TempDir()
	db, err := database.New(dataDir)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	h := &WorkspacesHandler{db: db, dataDir: dataDir}
	filesDir := h.filesDir(DefaultWorkspaceID)
	if err := os.MkdirAll(filesDir, 0o755); err != nil {
		t.Fatalf("mkdir files dir: %v", err)
	}
	return h, filesDir
}

func readFileReq(t *testing.T, h *WorkspacesHandler, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Get("/workspaces/{id}/file", h.ReadFile)
	req := httptest.NewRequest(http.MethodGet,
		"/workspaces/"+DefaultWorkspaceID+"/file?dir=&path="+path, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func writeFileReq(t *testing.T, h *WorkspacesHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Put("/workspaces/{id}/file", h.WriteFile)
	req := httptest.NewRequest(http.MethodPut,
		"/workspaces/"+DefaultWorkspaceID+"/file", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestReadFile_ReturnsContents(t *testing.T) {
	h, files := newTestWorkspacesHandler(t)
	os.WriteFile(filepath.Join(files, "notes.md"), []byte("line one\nline two\n"), 0o644)

	rec := readFileReq(t, h, "notes.md")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var out struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	decodeTestJSON(t, rec, &out)
	if out.Content != "line one\nline two\n" {
		t.Errorf("content = %q", out.Content)
	}
	if out.Name != "notes.md" {
		t.Errorf("name = %q, want notes.md", out.Name)
	}
}

// A binary would be corrupted by a round trip through a textarea, so it must
// never open in the first place.
func TestReadFile_RefusesBinary(t *testing.T) {
	h, files := newTestWorkspacesHandler(t)
	os.WriteFile(filepath.Join(files, "blob.bin"), []byte{0xff, 0xfe, 0x00, 0x01, 0x80}, 0o644)

	if rec := readFileReq(t, h, "blob.bin"); rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415 (%s)", rec.Code, rec.Body.String())
	}
}

func TestReadFile_RefusesOversized(t *testing.T) {
	h, files := newTestWorkspacesHandler(t)
	os.WriteFile(filepath.Join(files, "huge.txt"), make([]byte, maxEditableFileSize+1), 0o644)

	if rec := readFileReq(t, h, "huge.txt"); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413 (%s)", rec.Code, rec.Body.String())
	}
}

func TestReadFile_RefusesDirectory(t *testing.T) {
	h, files := newTestWorkspacesHandler(t)
	os.MkdirAll(filepath.Join(files, "sub"), 0o755)

	if rec := readFileReq(t, h, "sub"); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}

// The editor takes a caller-supplied relative path, so escaping the workspace
// is the failure that matters most here.
func TestFileEndpoints_RejectPathTraversal(t *testing.T) {
	h, files := newTestWorkspacesHandler(t)

	// A canary outside the workspace that traversal would reach if unclamped.
	outside := filepath.Join(filepath.Dir(filepath.Dir(files)), "outside.txt")
	os.WriteFile(outside, []byte("secret"), 0o644)

	for _, p := range []string{
		"..%2Foutside.txt",
		"..%2F..%2Foutside.txt",
		"..%2F..%2F..%2F..%2F..%2Fetc%2Fpasswd",
		"%2Fetc%2Fpasswd",
	} {
		rec := readFileReq(t, h, p)
		if rec.Code == http.StatusOK {
			t.Errorf("read %q returned 200 — escaped the workspace: %s", p, rec.Body.String())
		}
	}

	// And the write side must not create anything outside either.
	rec := writeFileReq(t, h, `{"dir":"","path":"../../escaped.txt","content":"x"}`)
	if rec.Code == http.StatusOK {
		t.Errorf("write escaped the workspace: %s", rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(filepath.Dir(files)), "escaped.txt")); err == nil {
		t.Error("write created a file outside the workspace")
	}

	if data, _ := os.ReadFile(outside); string(data) != "secret" {
		t.Error("the file outside the workspace was modified")
	}
}

func TestWriteFile_SavesAndPreservesMode(t *testing.T) {
	h, files := newTestWorkspacesHandler(t)
	script := filepath.Join(files, "run.sh")
	os.WriteFile(script, []byte("#!/bin/sh\necho hi\n"), 0o755)

	rec := writeFileReq(t, h, `{"dir":"","path":"run.sh","content":"#!/bin/sh\necho edited\n"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	data, _ := os.ReadFile(script)
	if string(data) != "#!/bin/sh\necho edited\n" {
		t.Errorf("content = %q", string(data))
	}

	// An executable that silently loses its executable bit on save is a broken
	// script the user won't notice until it fails to run.
	info, err := os.Stat(script)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v, want -rwxr-xr-x", info.Mode().Perm())
	}
}

// This endpoint backs an editor opened from an existing tree row. A path that
// no longer resolves means the file moved or was deleted, and recreating it
// would resurrect deleted content.
func TestWriteFile_RefusesToCreateNewFiles(t *testing.T) {
	h, files := newTestWorkspacesHandler(t)

	rec := writeFileReq(t, h, `{"dir":"","path":"brand-new.txt","content":"x"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (%s)", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(files, "brand-new.txt")); err == nil {
		t.Error("a new file was created")
	}
}

func TestWriteFile_RefusesDirectory(t *testing.T) {
	h, files := newTestWorkspacesHandler(t)
	os.MkdirAll(filepath.Join(files, "sub"), 0o755)

	if rec := writeFileReq(t, h, `{"dir":"","path":"sub","content":"x"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}
