package agents

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Bundled skill files — everything in a skill directory besides SKILL.md.
//
// A skill is a folder, not a single document. The system prompt hands the agent
// the folder's real path (see AssembleSystemPrompt), and the agent has file and
// shell tools, so a SKILL.md that says "run scripts/deploy.sh" or "read
// references/api.md" works exactly as written — provided those files are
// actually there. Nothing used to be able to put them there: create wrote one
// SKILL.md and the installer fetched one blob. These functions are what make
// the rest of a skill reachable.

// SkillFile is one file inside a skill directory.
type SkillFile struct {
	// Path is relative to the skill directory, always slash-separated:
	// "SKILL.md", "scripts/deploy.sh".
	Path string `json:"path"`
	Size int64  `json:"size"`
	// Editable is false for files we will not render as text in the browser.
	Editable bool `json:"editable"`
}

// maxEditableSize bounds what the editor will load. Skills are documents and
// scripts; anything larger is an asset the browser has no business rendering
// into a textarea.
const maxEditableSize = 512 * 1024

// binaryExtensions are served as listed-but-not-editable. Checked by extension
// rather than sniffing content: an asset is usually declared by its name, and
// guessing wrong on a large file means reading it just to refuse it.
var binaryExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	".ico": true, ".pdf": true, ".zip": true, ".gz": true, ".tar": true,
	".mp3": true, ".mp4": true, ".wav": true, ".mov": true, ".woff": true,
	".woff2": true, ".ttf": true, ".otf": true, ".so": true, ".dylib": true,
	".exe": true, ".bin": true, ".db": true, ".sqlite": true,
}

func isEditable(path string, size int64) bool {
	if size > maxEditableSize {
		return false
	}
	return !binaryExtensions[strings.ToLower(filepath.Ext(path))]
}

// resolveSkillPath maps a skill-relative path to an absolute one, refusing
// anything that would escape the skill directory.
//
// The relative path arrives from the client, so this is the only thing standing
// between the API and the rest of the disk. Cleaning alone is not enough —
// "../../id_rsa" cleans to a valid-looking relative path — so the result is
// checked to be inside the skill root after resolution.
func resolveSkillPath(dataDir, skillName, relPath string) (string, error) {
	if !IsValidSkillName(skillName) {
		return "", fmt.Errorf("invalid skill name: %s", skillName)
	}
	if strings.TrimSpace(relPath) == "" {
		return "", fmt.Errorf("path is required")
	}
	// Absolute paths are never valid here; they would ignore the root entirely.
	if filepath.IsAbs(relPath) || strings.HasPrefix(relPath, "/") {
		return "", fmt.Errorf("path must be relative to the skill")
	}

	root := filepath.Join(globalSkillsDir(dataDir), skillName)
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve skill dir: %w", err)
	}

	full := filepath.Join(rootAbs, filepath.FromSlash(relPath))
	// filepath.Join cleans, so compare the cleaned result against the root.
	if full != rootAbs && !strings.HasPrefix(full, rootAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes the skill directory")
	}
	return full, nil
}

// ListSkillFiles walks a skill directory and returns every file in it,
// SKILL.md first and the rest alphabetically — SKILL.md is the entry point and
// belongs at the top regardless of how the folder happens to sort.
func ListSkillFiles(dataDir, skillName string) ([]SkillFile, error) {
	if !IsValidSkillName(skillName) {
		return nil, fmt.Errorf("invalid skill name: %s", skillName)
	}
	root := filepath.Join(globalSkillsDir(dataDir), skillName)
	if _, err := os.Stat(root); err != nil {
		return nil, fmt.Errorf("skill not found: %s", skillName)
	}

	files := []SkillFile{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries rather than failing the listing
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		slashed := filepath.ToSlash(rel)
		files = append(files, SkillFile{
			Path:     slashed,
			Size:     info.Size(),
			Editable: isEditable(slashed, info.Size()),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk skill dir: %w", err)
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].Path == "SKILL.md" {
			return true
		}
		if files[j].Path == "SKILL.md" {
			return false
		}
		return files[i].Path < files[j].Path
	})
	return files, nil
}

// ReadSkillFile returns the contents of one file inside a skill.
func ReadSkillFile(dataDir, skillName, relPath string) (string, error) {
	full, err := resolveSkillPath(dataDir, skillName, relPath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", fmt.Errorf("file not found: %s", relPath)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", relPath)
	}
	if !isEditable(relPath, info.Size()) {
		return "", fmt.Errorf("%s is not a text file the editor can open", relPath)
	}
	content, err := os.ReadFile(full)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	return string(content), nil
}

// WriteSkillFile creates or overwrites a file inside a skill, creating any
// parent directories the path implies so "scripts/deploy.sh" works without a
// separate step to make scripts/.
//
// Scripts are written executable. A skill that ships a script expects to run
// it, and a mode-644 deploy.sh fails with "permission denied" at the one moment
// the agent is least able to explain why.
func WriteSkillFile(dataDir, skillName, relPath, content string) error {
	full, err := resolveSkillPath(dataDir, skillName, relPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}
	mode := fs.FileMode(0644)
	if isScript(relPath, content) {
		mode = 0755
	}
	if err := os.WriteFile(full, []byte(content), mode); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	// WriteFile does not change the mode of an existing file, so set it.
	return os.Chmod(full, mode)
}

// isScript reports whether a file should be executable — either it lives in
// scripts/, carries a script extension, or opens with a shebang.
func isScript(relPath, content string) bool {
	if strings.HasPrefix(strings.ToLower(filepath.ToSlash(relPath)), "scripts/") {
		return true
	}
	switch strings.ToLower(filepath.Ext(relPath)) {
	case ".sh", ".bash", ".zsh", ".py", ".rb", ".pl":
		return true
	}
	return strings.HasPrefix(content, "#!")
}

// DeleteSkillFile removes one file from a skill, then prunes any directories
// the removal left empty so deleting the last script does not leave an empty
// scripts/ behind.
//
// SKILL.md cannot be deleted: it is what makes the directory a skill, and a
// folder without one is invisible to both the listing and the agent.
func DeleteSkillFile(dataDir, skillName, relPath string) error {
	if filepath.ToSlash(relPath) == "SKILL.md" {
		return fmt.Errorf("SKILL.md cannot be deleted — it defines the skill")
	}
	full, err := resolveSkillPath(dataDir, skillName, relPath)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil {
		return fmt.Errorf("delete file: %w", err)
	}

	root := filepath.Join(globalSkillsDir(dataDir), skillName)
	rootAbs, _ := filepath.Abs(root)
	for dir := filepath.Dir(full); dir != rootAbs && strings.HasPrefix(dir, rootAbs); dir = filepath.Dir(dir) {
		// Stops at the first non-empty directory: Remove fails on those.
		if os.Remove(dir) != nil {
			break
		}
	}
	return nil
}
