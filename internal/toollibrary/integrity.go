package toollibrary

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openpaw/openpaw/internal/database"
)

type FileHash struct {
	Filename string `json:"filename"`
	Hash     string `json:"hash"`
	Size     int64  `json:"size"`
}

func HashSourceDir(toolDir string) (string, []FileHash, error) {
	var hashes []FileHash

	err := filepath.Walk(toolDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		rel, _ := filepath.Rel(toolDir, path)
		ext := filepath.Ext(rel)
		base := filepath.Base(rel)

		if ext == ".go" || base == "go.mod" || base == "go.sum" || base == "manifest.json" {
			hash, err := hashFile(path)
			if err != nil {
				return fmt.Errorf("hash %s: %w", rel, err)
			}
			hashes = append(hashes, FileHash{
				Filename: rel,
				Hash:     hash,
				Size:     info.Size(),
			})
		}
		return nil
	})
	if err != nil {
		return "", nil, err
	}

	sort.Slice(hashes, func(i, j int) bool {
		return hashes[i].Filename < hashes[j].Filename
	})

	h := sha256.New()
	for _, fh := range hashes {
		h.Write([]byte(fh.Filename + ":" + fh.Hash + "\n"))
	}
	overall := hex.EncodeToString(h.Sum(nil))

	return overall, hashes, nil
}

func HashBinary(toolDir string) (string, error) {
	binaryPath := filepath.Join(toolDir, "tool")
	return hashFile(binaryPath)
}

func VerifyIntegrity(toolDir, expectedSource, expectedBinary string) (bool, error) {
	sourceHash, _, err := HashSourceDir(toolDir)
	if err != nil {
		return false, fmt.Errorf("hash source: %w", err)
	}
	if sourceHash != expectedSource {
		return false, nil
	}

	if expectedBinary != "" {
		binaryHash, err := HashBinary(toolDir)
		if err != nil {
			return false, fmt.Errorf("hash binary: %w", err)
		}
		if binaryHash != expectedBinary {
			return false, nil
		}
	}

	return true, nil
}

func RecordIntegrity(db *database.DB, toolID, toolDir string) error {
	sourceHash, fileHashes, err := HashSourceDir(toolDir)
	if err != nil {
		return fmt.Errorf("hash source dir: %w", err)
	}

	binaryHash, err := HashBinary(toolDir)
	if err != nil {
		binaryHash = ""
	}

	now := time.Now().UTC()
	db.Exec("UPDATE tools SET source_hash = ?, binary_hash = ?, updated_at = ? WHERE id = ?",
		sourceHash, binaryHash, now, toolID)

	db.Exec("DELETE FROM tool_integrity WHERE tool_id = ?", toolID)

	for _, fh := range fileHashes {
		db.Exec(
			"INSERT INTO tool_integrity (id, tool_id, filename, file_hash, file_size, recorded_at) VALUES (?, ?, ?, ?, ?, ?)",
			uuid.New().String(), toolID, fh.Filename, fh.Hash, fh.Size, now,
		)
	}

	return nil
}

func GetIntegrity(db *database.DB, toolID string) ([]FileHash, error) {
	rows, err := db.Query(
		"SELECT filename, file_hash, file_size FROM tool_integrity WHERE tool_id = ? ORDER BY filename",
		toolID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hashes []FileHash
	for rows.Next() {
		var fh FileHash
		if err := rows.Scan(&fh.Filename, &fh.Hash, &fh.Size); err != nil {
			return nil, err
		}
		hashes = append(hashes, fh)
	}
	return hashes, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func IsTampered(toolDir string, expectedSourceHash string) bool {
	if expectedSourceHash == "" {
		return false
	}
	currentHash, _, err := HashSourceDir(toolDir)
	if err != nil {
		return true
	}
	return !strings.EqualFold(currentHash, expectedSourceHash)
}
