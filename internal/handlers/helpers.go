package handlers

import (
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strings"
)

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func decodeJSON(r *http.Request, v interface{}) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20) // 1MB limit
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// validateImageMagicBytes checks that the file content matches the expected
// image type based on the file extension. The file position is reset after reading.
func validateImageMagicBytes(file multipart.File, ext string) bool {
	buf := make([]byte, 12)
	n, _ := file.Read(buf)
	buf = buf[:n]
	file.Seek(0, 0) // Reset for later io.Copy

	switch ext {
	case ".png":
		return n >= 4 && buf[0] == 0x89 && buf[1] == 0x50 && buf[2] == 0x4E && buf[3] == 0x47
	case ".jpg":
		return n >= 3 && buf[0] == 0xFF && buf[1] == 0xD8 && buf[2] == 0xFF
	case ".webp":
		return n >= 12 && string(buf[0:4]) == "RIFF" && string(buf[8:12]) == "WEBP"
	}
	return false
}

// escapeLike escapes SQL LIKE wildcard characters (% and _) in a string.
func escapeLike(s string) string {
	r := strings.NewReplacer("%", "\\%", "_", "\\_")
	return r.Replace(s)
}
