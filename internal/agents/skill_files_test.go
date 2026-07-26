package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newSkillDir(t *testing.T) string {
	t.Helper()
	dataDir := t.TempDir()
	if err := WriteGlobalSkill(dataDir, "railway", "---\nname: railway\n---\n\n# Railway\n"); err != nil {
		t.Fatalf("seed skill: %v", err)
	}
	return dataDir
}

// The relative path comes straight from the client, so this is the only thing
// between the API and the rest of the disk. Cleaning is not enough on its own —
// "../../x" cleans to something that still looks like a relative path.
func TestSkillFiles_RejectPathEscape(t *testing.T) {
	dataDir := newSkillDir(t)

	// A file outside the skill that must stay untouched.
	outside := filepath.Join(dataDir, "secret.txt")
	if err := os.WriteFile(outside, []byte("original"), 0644); err != nil {
		t.Fatalf("seed outside file: %v", err)
	}

	escapes := []string{
		"../secret.txt",
		"../../secret.txt",
		"scripts/../../secret.txt",
		"/etc/passwd",
		"./../secret.txt",
	}

	for _, p := range escapes {
		if _, err := ReadSkillFile(dataDir, "railway", p); err == nil {
			t.Errorf("ReadSkillFile(%q) succeeded, want refusal", p)
		}
		if err := WriteSkillFile(dataDir, "railway", p, "pwned"); err == nil {
			t.Errorf("WriteSkillFile(%q) succeeded, want refusal", p)
		}
		if err := DeleteSkillFile(dataDir, "railway", p); err == nil {
			t.Errorf("DeleteSkillFile(%q) succeeded, want refusal", p)
		}
	}

	if got, _ := os.ReadFile(outside); string(got) != "original" {
		t.Fatalf("file outside the skill was modified: %q", got)
	}
}

func TestSkillFiles_RejectBadSkillName(t *testing.T) {
	dataDir := newSkillDir(t)
	for _, name := range []string{"../other", "rail/way", ""} {
		if _, err := ListSkillFiles(dataDir, name); err == nil {
			t.Errorf("ListSkillFiles(%q) succeeded, want refusal", name)
		}
		if err := WriteSkillFile(dataDir, name, "a.md", "x"); err == nil {
			t.Errorf("WriteSkillFile with skill %q succeeded, want refusal", name)
		}
	}
}

func TestWriteAndListSkillFiles(t *testing.T) {
	dataDir := newSkillDir(t)

	// Nested path with no existing parent — the directory has to be created.
	if err := WriteSkillFile(dataDir, "railway", "scripts/deploy.sh", "#!/bin/sh\necho deploying\n"); err != nil {
		t.Fatalf("write nested file: %v", err)
	}
	if err := WriteSkillFile(dataDir, "railway", "references/api.md", "# API\n"); err != nil {
		t.Fatalf("write reference: %v", err)
	}

	files, err := ListSkillFiles(dataDir, "railway")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("got %d files, want 3: %+v", len(files), files)
	}
	// SKILL.md is the entry point and must lead regardless of alphabetical order.
	if files[0].Path != "SKILL.md" {
		t.Errorf("first file = %q, want SKILL.md", files[0].Path)
	}

	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	joined := strings.Join(paths, ",")
	if !strings.Contains(joined, "scripts/deploy.sh") || !strings.Contains(joined, "references/api.md") {
		t.Errorf("listing missing nested files: %v", paths)
	}

	got, err := ReadSkillFile(dataDir, "railway", "scripts/deploy.sh")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !strings.Contains(got, "echo deploying") {
		t.Errorf("content = %q", got)
	}
}

// A skill that ships a script expects to run it; mode 644 fails with
// "permission denied" at the one moment the agent can least explain why.
func TestWriteSkillFile_ScriptsAreExecutable(t *testing.T) {
	dataDir := newSkillDir(t)

	if err := WriteSkillFile(dataDir, "railway", "scripts/deploy.sh", "#!/bin/sh\ntrue\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	info, err := os.Stat(filepath.Join(dataDir, "skills", "railway", "scripts", "deploy.sh"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Errorf("mode = %v, want the execute bit set", info.Mode())
	}

	// A plain reference must not become executable.
	if err := WriteSkillFile(dataDir, "railway", "references/api.md", "# API\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	refInfo, err := os.Stat(filepath.Join(dataDir, "skills", "railway", "references", "api.md"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if refInfo.Mode()&0111 != 0 {
		t.Errorf("reference mode = %v, want no execute bit", refInfo.Mode())
	}
}

// SKILL.md is what makes the folder a skill — without one it disappears from
// the listing and from the agent's prompt entirely.
func TestDeleteSkillFile_RefusesSkillMd(t *testing.T) {
	dataDir := newSkillDir(t)
	if err := DeleteSkillFile(dataDir, "railway", "SKILL.md"); err == nil {
		t.Fatal("deleting SKILL.md succeeded, want refusal")
	}
	if _, err := GetGlobalSkill(dataDir, "railway"); err != nil {
		t.Fatalf("SKILL.md was removed anyway: %v", err)
	}
}

func TestDeleteSkillFile_PrunesEmptyDirs(t *testing.T) {
	dataDir := newSkillDir(t)
	if err := WriteSkillFile(dataDir, "railway", "scripts/deploy.sh", "#!/bin/sh\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := DeleteSkillFile(dataDir, "railway", "scripts/deploy.sh"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "skills", "railway", "scripts")); !os.IsNotExist(err) {
		t.Error("empty scripts/ left behind after deleting its last file")
	}
	// The skill itself must survive.
	if _, err := GetGlobalSkill(dataDir, "railway"); err != nil {
		t.Fatalf("skill was pruned too: %v", err)
	}
}

// Binaries are listed so the folder's real contents are visible, but the editor
// must not try to load them into a textarea.
func TestSkillFiles_BinariesListedNotEditable(t *testing.T) {
	dataDir := newSkillDir(t)
	assetDir := filepath.Join(dataDir, "skills", "railway", "assets")
	if err := os.MkdirAll(assetDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "logo.png"), []byte{0x89, 'P', 'N', 'G'}, 0644); err != nil {
		t.Fatalf("write png: %v", err)
	}

	files, err := ListSkillFiles(dataDir, "railway")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var found bool
	for _, f := range files {
		if f.Path == "assets/logo.png" {
			found = true
			if f.Editable {
				t.Error("png reported as editable")
			}
		}
	}
	if !found {
		t.Error("png missing from listing — the folder's contents should be visible")
	}
	if _, err := ReadSkillFile(dataDir, "railway", "assets/logo.png"); err == nil {
		t.Error("reading a png succeeded, want refusal")
	}
}
