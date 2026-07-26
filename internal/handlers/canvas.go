package handlers

import (
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// CanvasHandler serves local files to the chat preview canvas.
//
// The canvas is an iframe, and an iframe cannot load file:// from an https/http
// document — so previewing a local build means serving it. Files come back
// under /api/v1/canvas/fs/<absolute path>, which keeps a page's relative links
// working: ./style.css next to /Users/me/site/index.html resolves to
// /api/v1/canvas/fs/Users/me/site/style.css, which is the same file.
//
// Everything here is behind the normal auth middleware, and every response is
// sandboxed (see below) so a previewed page cannot reach back into OpenPaw.
type CanvasHandler struct{}

func NewCanvasHandler() *CanvasHandler { return &CanvasHandler{} }

// CanvasFSPrefix is the URL prefix the canvas serves local files under. The
// agent-facing canvas tool builds URLs with it too.
const CanvasFSPrefix = "/api/v1/canvas/fs/"

// CanvasURLForPath turns an absolute filesystem path into a canvas URL.
func CanvasURLForPath(abs string) string {
	// Each segment is escaped separately so "/" stays a separator.
	parts := strings.Split(strings.TrimPrefix(filepath.ToSlash(abs), "/"), "/")
	for i, p := range parts {
		parts[i] = (&url.URL{Path: p}).EscapedPath()
	}
	return CanvasFSPrefix + strings.Join(parts, "/")
}

// ServeFile serves one local file to the canvas iframe.
func (h *CanvasHandler) ServeFile(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, CanvasFSPrefix)
	if rest == "" {
		writeError(w, http.StatusBadRequest, "no path given")
		return
	}

	// r.URL.Path is already percent-decoded by net/http. Clean rejects any ".."
	// that would climb out of the path it was handed.
	abs := filepath.Clean("/" + rest)
	if strings.Contains(abs, "..") {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}

	info, err := os.Stat(abs)
	if err != nil {
		writeError(w, http.StatusNotFound, "no such file: "+abs)
		return
	}
	if info.IsDir() {
		// A directory is almost always a site root — serve its index rather than
		// a listing, which is what the user asked to look at.
		index := filepath.Join(abs, "index.html")
		if _, err := os.Stat(index); err != nil {
			writeError(w, http.StatusNotFound, "no index.html in "+abs)
			return
		}
		http.Redirect(w, r, CanvasURLForPath(index), http.StatusFound)
		return
	}

	f, err := os.Open(abs)
	if err != nil {
		writeError(w, http.StatusForbidden, "cannot read "+abs)
		return
	}
	defer f.Close()

	// Go's mime table is thin on the web types that matter most here, and a
	// wrong type on a module script is a blank page rather than an error.
	ext := strings.ToLower(filepath.Ext(abs))
	var contentType string
	switch ext {
	case ".js", ".mjs":
		contentType = "text/javascript; charset=utf-8"
	case ".css":
		contentType = "text/css; charset=utf-8"
	case ".html", ".htm":
		contentType = "text/html; charset=utf-8"
	case ".json":
		contentType = "application/json; charset=utf-8"
	case ".svg":
		contentType = "image/svg+xml"
	default:
		if contentType = mime.TypeByExtension(ext); contentType == "" {
			contentType = "application/octet-stream"
		}
	}
	w.Header().Set("Content-Type", contentType)

	// The global middleware sets X-Frame-Options: DENY, which would stop our own
	// page framing this. Same origin only, which is all the canvas needs.
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")

	// `sandbox` in a response header puts the document in an opaque origin. That
	// matters because this is served from OpenPaw's own origin: without it, a
	// previewed page would be same-origin with the app and could read its
	// cookies and call its API. Scripts still run, so a local build is still
	// worth looking at.
	w.Header().Set("Content-Security-Policy", "sandbox allow-scripts allow-forms allow-popups allow-modals")
	w.Header().Set("Cache-Control", "no-store")

	// Not logged: a single page pulls in every asset beside it, and one line per
	// request would bury the log.
	http.ServeContent(w, r, filepath.Base(abs), info.ModTime(), f)
}
