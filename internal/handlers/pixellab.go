package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
	"github.com/openpaw/openpaw/internal/logger"
	"github.com/openpaw/openpaw/internal/middleware"
	"github.com/openpaw/openpaw/internal/secrets"
)

// pixelLabBaseURL is the only upstream the proxy will ever talk to.
const pixelLabBaseURL = "https://api.pixellab.ai/v2"

const pixelLabKeySetting = "pixellab_api_key"

// pixelLabHTTP has a generous timeout — image generation and character creation
// can take a while, and the client polls long-running jobs through this proxy.
var pixelLabHTTP = &http.Client{Timeout: 90 * time.Second}

// PixelLabHandler stores the encrypted PixelLab API key, proxies requests to the
// PixelLab API (so the key never reaches the browser), and persists generated
// pixel-art characters (sprite frames on disk + a metadata row).
type PixelLabHandler struct {
	db         *database.DB
	secretsMgr *secrets.Manager
	dataDir    string
}

func NewPixelLabHandler(db *database.DB, secretsMgr *secrets.Manager, dataDir string) *PixelLabHandler {
	return &PixelLabHandler{db: db, secretsMgr: secretsMgr, dataDir: dataDir}
}

// spritesDir is where all character frames live on disk.
func (h *PixelLabHandler) spritesDir() string {
	return filepath.Join(h.dataDir, "pixellab")
}

// decryptedKey returns the stored PixelLab key, or "" if none/undecryptable.
func (h *PixelLabHandler) decryptedKey() string {
	var enc string
	if err := h.db.QueryRow("SELECT value FROM settings WHERE key = ?", pixelLabKeySetting).Scan(&enc); err != nil || enc == "" {
		return ""
	}
	key, err := h.secretsMgr.Decrypt(enc)
	if err != nil {
		logger.Error("pixellab: failed to decrypt api key: %v", err)
		return ""
	}
	return key
}

// ---------------------------------------------------------------------------
// API key
// ---------------------------------------------------------------------------

func (h *PixelLabHandler) GetAPIKey(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"configured": h.decryptedKey() != "",
	})
}

func (h *PixelLabHandler) UpdateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIKey string `json:"api_key"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.APIKey = strings.TrimSpace(req.APIKey)
	if req.APIKey == "" {
		writeError(w, http.StatusBadRequest, "api_key is required")
		return
	}

	// Validate the key with a lightweight balance check; reject only on auth
	// failure so transient network issues don't block saving.
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	if status := pixelLabBalanceStatus(ctx, req.APIKey); status == http.StatusUnauthorized {
		writeError(w, http.StatusBadRequest, "Invalid PixelLab API key")
		return
	}

	encrypted, err := h.secretsMgr.Encrypt(req.APIKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encrypt API key")
		return
	}
	h.upsert(pixelLabKeySetting, encrypted)

	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "pixellab_key_updated", "settings", "settings", pixelLabKeySetting, "")

	writeJSON(w, http.StatusOK, map[string]interface{}{"configured": true})
}

func pixelLabBalanceStatus(ctx context.Context, apiKey string) int {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pixelLabBaseURL+"/balance", nil)
	if err != nil {
		return 0
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := pixelLabHTTP.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func (h *PixelLabHandler) upsert(key, value string) {
	if _, err := h.db.Exec(
		"INSERT INTO settings (id, key, value) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		uuid.New().String(), key, value,
	); err != nil {
		logger.Error("pixellab: failed to upsert setting %s: %v", key, err)
	}
}

// ---------------------------------------------------------------------------
// Proxy — forwards an allow-listed PixelLab path with the stored key injected
// ---------------------------------------------------------------------------

func pixelLabPathAllowed(path string) bool {
	if path == "" || strings.Contains(path, "://") || strings.Contains(path, "..") {
		return false
	}
	switch {
	case path == "/balance":
		return true
	case strings.HasPrefix(path, "/create-image-pixflux"):
		return true
	case strings.HasPrefix(path, "/create-character-v3"):
		return true
	case strings.HasPrefix(path, "/animate-character"):
		return true
	case strings.HasPrefix(path, "/background-jobs/"):
		return true
	}
	return false
}

func (h *PixelLabHandler) Proxy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path   string          `json:"path"`
		Method string          `json:"method"`
		Body   json.RawMessage `json:"body"`
	}
	// Allow large bodies — reference_image payloads are base64 sprites.
	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !pixelLabPathAllowed(req.Path) {
		writeError(w, http.StatusBadRequest, "path not allowed")
		return
	}
	method := strings.ToUpper(req.Method)
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodPost {
		writeError(w, http.StatusBadRequest, "method not allowed")
		return
	}

	apiKey := h.decryptedKey()
	if apiKey == "" {
		writeError(w, http.StatusBadRequest, "PixelLab API key not configured")
		return
	}

	var bodyReader io.Reader
	if len(req.Body) > 0 && string(req.Body) != "null" {
		bodyReader = bytes.NewReader(req.Body)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	upReq, err := http.NewRequestWithContext(ctx, method, pixelLabBaseURL+req.Path, bodyReader)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build upstream request")
		return
	}
	upReq.Header.Set("Authorization", "Bearer "+apiKey)
	upReq.Header.Set("Content-Type", "application/json")

	resp, err := pixelLabHTTP.Do(upReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, "PixelLab request failed: "+err.Error())
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 24<<20))

	// Envelope the upstream status + body so the proxy itself always returns 200
	// (unless the proxy failed) and the client can map PixelLab error codes
	// (401/402/422/429) without api.ts treating them as transport errors.
	data := json.RawMessage(respBody)
	if !json.Valid(respBody) {
		quoted, _ := json.Marshal(string(respBody))
		data = json.RawMessage(quoted)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": resp.StatusCode,
		"data":   data,
	})
}

// ---------------------------------------------------------------------------
// Characters — persistence
// ---------------------------------------------------------------------------

// storedClip is the on-disk manifest entry for an animation (relative frame paths).
type storedClip struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	FPS    int      `json:"fps"`
	Frames []string `json:"frames"`
}

// clipInput is one animation in a save/add request (base64 data-URI frames).
type clipInput struct {
	Name   string   `json:"name"`
	FPS    int      `json:"fps"`
	Frames []string `json:"frames"`
}

// clipOut is an animation returned to the client (frames resolved to URLs).
type clipOut struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	FPS    int      `json:"fps"`
	Frames []string `json:"frames"`
}

type charOut struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	PixellabID string    `json:"pixellab_id"`
	BaseURL    string    `json:"base_url"`
	Animations []clipOut `json:"animations"`
	Pinned     bool      `json:"pinned"`
	AgentSlug  string    `json:"agent_slug"`
	CreatedAt  string    `json:"created_at"`
}

// spriteURLPrefix is the public path under which stored frames are served.
const spriteURLPrefix = "/api/v1/pixellab/sprites/"

func (h *PixelLabHandler) frameURL(charID, relPath string) string {
	return spriteURLPrefix + charID + "/" + relPath
}

// spriteFilePath maps a stored-sprite URL ("/api/v1/pixellab/sprites/{char}/{rel}")
// to its file on disk, or returns "" if s is not such a URL. It rejects paths that
// would escape the sprites directory.
func (h *PixelLabHandler) spriteFilePath(s string) string {
	if !strings.HasPrefix(s, spriteURLPrefix) {
		return ""
	}
	rel := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(s, spriteURLPrefix)))
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return ""
	}
	base := h.spritesDir()
	full := filepath.Join(base, rel)
	if full != base && !strings.HasPrefix(full, base+string(os.PathSeparator)) {
		return ""
	}
	return full
}

// decodeDataURI turns a base64 (optionally data:-prefixed) string into bytes.
func decodeDataURI(s string) ([]byte, error) {
	if strings.HasPrefix(s, "data:") {
		if i := strings.IndexByte(s, ','); i >= 0 {
			s = s[i+1:]
		}
	}
	return base64.StdEncoding.DecodeString(strings.TrimSpace(s))
}

// pngMagic is the 8-byte signature every PNG file begins with.
var pngMagic = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}

// saveFrames writes a clip's frames under {sprites}/{charID}/{clipID}/ and
// returns their relative ("{clipID}/frame_N.png") paths. Non-PNG payloads are
// skipped rather than written: PixelLab responses can include base64 blobs that
// aren't renderable images, and writing those produced "broken image" frames.
func (h *PixelLabHandler) saveFrames(charID, clipID string, frames []string) ([]string, error) {
	dir := filepath.Join(h.spritesDir(), charID, clipID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	rel := make([]string, 0, len(frames))
	for _, f := range frames {
		// A frame is either a freshly generated base64 (data:) PNG, or a reference
		// to an already-stored sprite ("/api/v1/pixellab/sprites/{char}/{path}")
		// when the client reuses an existing frame as a fallback. Resolve the
		// latter from disk instead of trying to base64-decode the URL.
		var bytesPNG []byte
		if src := h.spriteFilePath(f); src != "" {
			b, err := os.ReadFile(src)
			if err != nil {
				return nil, err
			}
			bytesPNG = b
		} else {
			b, err := decodeDataURI(f)
			if err != nil {
				return nil, err
			}
			bytesPNG = b
		}
		if !bytes.HasPrefix(bytesPNG, pngMagic) {
			logger.Error("pixellab: skipping non-PNG frame for char %s clip %s (%d bytes)", charID, clipID, len(bytesPNG))
			continue
		}
		name := "frame_" + strconv.Itoa(len(rel)) + ".png"
		if err := os.WriteFile(filepath.Join(dir, name), bytesPNG, 0o644); err != nil {
			return nil, err
		}
		rel = append(rel, clipID+"/"+name)
	}
	return rel, nil
}

func (h *PixelLabHandler) rowToOut(id, name, pixellabID, basePath, animsJSON string, pinned int, agentSlug, createdAt string) charOut {
	var stored []storedClip
	_ = json.Unmarshal([]byte(animsJSON), &stored)
	anims := make([]clipOut, 0, len(stored))
	for _, c := range stored {
		urls := make([]string, 0, len(c.Frames))
		for _, rel := range c.Frames {
			urls = append(urls, h.frameURL(id, rel))
		}
		anims = append(anims, clipOut{ID: c.ID, Name: c.Name, FPS: c.FPS, Frames: urls})
	}
	base := ""
	if basePath != "" {
		base = h.frameURL(id, basePath)
	}
	return charOut{
		ID: id, Name: name, PixellabID: pixellabID, BaseURL: base,
		Animations: anims, Pinned: pinned != 0, AgentSlug: agentSlug, CreatedAt: createdAt,
	}
}

func (h *PixelLabHandler) ListCharacters(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query("SELECT id, name, pixellab_id, base_path, animations, pinned, agent_slug, created_at FROM pixellab_characters ORDER BY created_at ASC")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list characters")
		return
	}
	defer rows.Close()

	out := []charOut{}
	for rows.Next() {
		var id, name, pixellabID, basePath, anims, agentSlug, createdAt string
		var pinned int
		if err := rows.Scan(&id, &name, &pixellabID, &basePath, &anims, &pinned, &agentSlug, &createdAt); err != nil {
			continue
		}
		out = append(out, h.rowToOut(id, name, pixellabID, basePath, anims, pinned, agentSlug, createdAt))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *PixelLabHandler) CreateCharacter(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string      `json:"name"`
		PixellabID string      `json:"pixellab_id"`
		BaseSprite string      `json:"base_sprite"`
		Animations []clipInput `json:"animations"`
		AgentSlug  string      `json:"agent_slug"`
	}
	// Frames are base64 PNGs — allow a large body.
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		req.Name = "Companion"
	}

	id := uuid.New().String()

	basePath := ""
	if req.BaseSprite != "" {
		rel, err := h.saveFrames(id, "base", []string{req.BaseSprite})
		if err != nil {
			logger.Error("pixellab: save base sprite for char %s: %v", id, err)
			writeError(w, http.StatusInternalServerError, "failed to save base sprite")
			return
		}
		if len(rel) > 0 {
			basePath = rel[0]
		}
	}

	stored := make([]storedClip, 0, len(req.Animations))
	for _, c := range req.Animations {
		clipID := uuid.New().String()
		rel, err := h.saveFrames(id, clipID, c.Frames)
		if err != nil {
			logger.Error("pixellab: save clip %q for char %s: %v", c.Name, id, err)
			os.RemoveAll(filepath.Join(h.spritesDir(), id))
			writeError(w, http.StatusInternalServerError, "failed to save animation frames")
			return
		}
		fps := c.FPS
		if fps <= 0 {
			fps = 6
		}
		stored = append(stored, storedClip{ID: clipID, Name: c.Name, FPS: fps, Frames: rel})
	}

	animsJSON, _ := json.Marshal(stored)
	userID := middleware.GetUserID(r.Context())
	if _, err := h.db.Exec(
		"INSERT INTO pixellab_characters (id, user_id, name, pixellab_id, base_path, animations, pinned, agent_slug) VALUES (?, ?, ?, ?, ?, ?, 0, ?)",
		id, userID, req.Name, req.PixellabID, basePath, string(animsJSON), req.AgentSlug,
	); err != nil {
		os.RemoveAll(filepath.Join(h.spritesDir(), id))
		writeError(w, http.StatusInternalServerError, "failed to save character")
		return
	}

	h.db.LogAudit(userID, "pixellab_character_created", "pixellab", "character", id, req.Name)
	writeJSON(w, http.StatusOK, h.rowToOut(id, req.Name, req.PixellabID, basePath, string(animsJSON), 0, req.AgentSlug, ""))
}

func (h *PixelLabHandler) AddAnimation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req clipInput
	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var animsJSON string
	if err := h.db.QueryRow("SELECT animations FROM pixellab_characters WHERE id = ?", id).Scan(&animsJSON); err != nil {
		writeError(w, http.StatusNotFound, "character not found")
		return
	}
	var stored []storedClip
	_ = json.Unmarshal([]byte(animsJSON), &stored)

	clipID := uuid.New().String()
	rel, err := h.saveFrames(id, clipID, req.Frames)
	if err != nil {
		logger.Error("pixellab: save clip %q for char %s: %v", req.Name, id, err)
		writeError(w, http.StatusInternalServerError, "failed to save animation frames")
		return
	}
	fps := req.FPS
	if fps <= 0 {
		fps = 6
	}
	// Replace a same-named clip if present, else append.
	filtered := stored[:0]
	for _, c := range stored {
		if c.Name != req.Name {
			filtered = append(filtered, c)
		}
	}
	stored = append(filtered, storedClip{ID: clipID, Name: req.Name, FPS: fps, Frames: rel})

	newJSON, _ := json.Marshal(stored)
	if _, err := h.db.Exec("UPDATE pixellab_characters SET animations = ? WHERE id = ?", string(newJSON), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update character")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"id": clipID, "name": req.Name, "fps": fps})
}

func (h *PixelLabHandler) UpdateCharacter(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Pinned    *bool   `json:"pinned"`
		AgentSlug *string `json:"agent_slug"`
		Name      *string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Pinned != nil {
		v := 0
		if *req.Pinned {
			v = 1
		}
		h.db.Exec("UPDATE pixellab_characters SET pinned = ? WHERE id = ?", v, id)
	}
	if req.AgentSlug != nil {
		h.db.Exec("UPDATE pixellab_characters SET agent_slug = ? WHERE id = ?", *req.AgentSlug, id)
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		h.db.Exec("UPDATE pixellab_characters SET name = ? WHERE id = ?", *req.Name, id)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *PixelLabHandler) DeleteCharacter(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.db.Exec("DELETE FROM pixellab_characters WHERE id = ?", id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete character")
		return
	}
	os.RemoveAll(filepath.Join(h.spritesDir(), filepath.Base(id)))
	userID := middleware.GetUserID(r.Context())
	h.db.LogAudit(userID, "pixellab_character_deleted", "pixellab", "character", id, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ServeFrame serves a sprite PNG from disk. The path is {id}/{clip}/frame_N.png
// relative to the sprites dir; every segment is sanitized against traversal.
func (h *PixelLabHandler) ServeFrame(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rest := chi.URLParam(r, "*")
	if id == "" || rest == "" || strings.Contains(id, "..") || strings.Contains(rest, "..") {
		http.NotFound(w, r)
		return
	}
	// Rebuild the path from sanitized segments.
	clean := filepath.Join(h.spritesDir(), filepath.Base(id))
	for _, seg := range strings.Split(rest, "/") {
		if seg == "" {
			continue
		}
		clean = filepath.Join(clean, filepath.Base(seg))
	}
	if info, err := os.Stat(clean); err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, clean)
}
