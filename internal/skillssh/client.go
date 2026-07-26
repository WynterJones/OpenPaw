package skillssh

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

type SkillResult struct {
	ID       string `json:"id"`
	SkillID  string `json:"skillId"`
	Name     string `json:"name"`
	Installs int    `json:"installs"`
	Source   string `json:"source"`
}

type searchResponse struct {
	Skills []SkillResult `json:"skills"`
}

type SkillDetail struct {
	SkillID     string `json:"skillId"`
	Name        string `json:"name"`
	Source      string `json:"source"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

type cacheEntry struct {
	data      []SkillResult
	expiresAt time.Time
}

type repoEntry struct {
	branch string
	paths  []string // every "…/SKILL.md" in the repo
	// blobs is every file in the repo. The recursive tree call already returns
	// them, and a skill is a directory — its scripts/ and references/ are what
	// make it more than a prompt, so they have to be findable.
	blobs     []string
	truncated bool
	expiresAt time.Time
}

type Client struct {
	httpClient *http.Client
	mu         sync.RWMutex
	cache      map[string]cacheEntry
	pathCache  map[string]string    // "source/skillId" -> full in-repo path to SKILL.md
	repoCache  map[string]repoEntry // "owner/repo"      -> branch + SKILL.md paths
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		cache:      make(map[string]cacheEntry),
		pathCache:  make(map[string]string),
		repoCache:  make(map[string]repoEntry),
	}
}

func (c *Client) Search(query string) ([]SkillResult, error) {
	if query == "" {
		query = "agent"
	}

	c.mu.RLock()
	if entry, ok := c.cache[query]; ok && time.Now().Before(entry.expiresAt) {
		c.mu.RUnlock()
		return entry.data, nil
	}
	c.mu.RUnlock()

	u := fmt.Sprintf("https://skills.sh/api/search?q=%s", url.QueryEscape(query))
	resp, err := c.httpClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("skills.sh search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("skills.sh returned %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var envelope searchResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	results := envelope.Skills

	c.mu.Lock()
	c.cache[query] = cacheEntry{data: results, expiresAt: time.Now().Add(5 * time.Minute)}
	c.mu.Unlock()

	return results, nil
}

func (c *Client) fetchRaw(rawURL string) (string, int, error) {
	resp, err := c.httpClient.Get(rawURL)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", resp.StatusCode, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, err
	}
	return string(body), resp.StatusCode, nil
}

// FetchSkillContent returns the SKILL.md body for a published skill.
//
// The path is resolved from the repository's actual file tree rather than
// assumed. The previous version built "skills/<skillId>/SKILL.md" on the main
// branch, which only holds for flat repos like anthropics/skills. Real
// catalogues nest by category — the published `google-ads` skill lives at
// skills/paid-ads/platforms/google-ads/SKILL.md — so every non-flat repo 404'd,
// the fetch reported "no published skill", and the caller silently wrote the
// model's own improvised stub instead of the real thing.
func (c *Client) FetchSkillContent(source, skillID string) (string, error) {
	cacheKey := source + "/" + skillID

	c.mu.RLock()
	path, cached := c.pathCache[cacheKey]
	c.mu.RUnlock()

	if cached {
		if content, ok := c.fetchAtPath(source, path); ok {
			return content, nil
		}
	}

	path, err := c.resolveSkillPath(source, skillID)
	if err != nil {
		return "", err
	}

	content, ok := c.fetchAtPath(source, path)
	if !ok {
		return "", fmt.Errorf("fetch %s/%s: raw content unavailable", source, path)
	}

	c.mu.Lock()
	c.pathCache[cacheKey] = path
	c.mu.Unlock()
	return content, nil
}

// fetchAtPath reads one file from a repo, trying the resolved default branch
// first. The branch comes from the repo listing, so "main" is not assumed.
func (c *Client) fetchAtPath(source, path string) (string, bool) {
	branch := "main"
	if repo, ok := c.repo(source); ok && repo.branch != "" {
		branch = repo.branch
	}
	rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", source, branch, path)
	content, status, err := c.fetchRaw(rawURL)
	if err != nil || status != http.StatusOK || strings.TrimSpace(content) == "" {
		return "", false
	}
	return content, true
}

// resolveSkillPath finds the in-repo path of a skill's SKILL.md.
//
// Matching is on the containing directory name first — that is how skills are
// named in every catalogue seen so far and it costs no extra requests. Only if
// that fails does it read frontmatter, which needs one fetch per candidate.
func (c *Client) resolveSkillPath(source, skillID string) (string, error) {
	repo, ok := c.repo(source)
	if !ok {
		return "", fmt.Errorf("could not list %s", source)
	}
	if len(repo.paths) == 0 {
		if repo.truncated {
			return "", fmt.Errorf("%s is too large to list", source)
		}
		return "", fmt.Errorf("no SKILL.md files in %s", source)
	}

	want := SanitizeSkillName(skillID)

	for _, p := range repo.paths {
		if SanitizeSkillName(dirNameOf(p)) == want {
			return p, nil
		}
	}

	// Fall back to frontmatter, since a skill's declared name need not match
	// its directory. Bounded: an unbounded scan of a 170-file catalogue would
	// exhaust GitHub's unauthenticated rate limit on a single lookup.
	const maxProbes = 25
	probes := 0
	for _, p := range repo.paths {
		if probes >= maxProbes {
			break
		}
		probes++
		content, ok := c.fetchAtPath(source, p)
		if !ok {
			continue
		}
		if SanitizeSkillName(parseFrontmatterName(content)) == want {
			return p, nil
		}
	}

	return "", fmt.Errorf("skill %q not found in %s", skillID, source)
}

// dirNameOf returns the directory holding a file — "a/b/c/SKILL.md" -> "c".
func dirNameOf(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2]
}

type ghRepo struct {
	DefaultBranch string `json:"default_branch"`
}

type ghTree struct {
	Tree []struct {
		Path string `json:"path"`
		Type string `json:"type"`
	} `json:"tree"`
	Truncated bool `json:"truncated"`
}

// repo lists a repository's SKILL.md files, cached for 15 minutes.
//
// One recursive tree request covers the whole repo. The old approach walked
// skills/ one directory at a time and fetched a SKILL.md per entry, which both
// missed anything nested deeper and burned GitHub's 60-requests-per-hour
// unauthenticated budget on a single lookup.
func (c *Client) repo(source string) (repoEntry, bool) {
	c.mu.RLock()
	entry, ok := c.repoCache[source]
	c.mu.RUnlock()
	if ok && time.Now().Before(entry.expiresAt) {
		return entry, true
	}

	branch := c.defaultBranch(source)
	if branch == "" {
		return repoEntry{}, false
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/git/trees/%s?recursive=1", source, url.PathEscape(branch))
	resp, err := c.httpClient.Get(apiURL)
	if err != nil {
		return repoEntry{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return repoEntry{}, false
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return repoEntry{}, false
	}

	var tree ghTree
	if err := json.Unmarshal(body, &tree); err != nil {
		return repoEntry{}, false
	}

	var paths, blobs []string
	for _, t := range tree.Tree {
		if t.Type != "blob" {
			continue
		}
		blobs = append(blobs, t.Path)
		if strings.HasSuffix(t.Path, "/SKILL.md") {
			paths = append(paths, t.Path)
		}
	}

	entry = repoEntry{
		branch:    branch,
		paths:     paths,
		blobs:     blobs,
		truncated: tree.Truncated,
		expiresAt: time.Now().Add(15 * time.Minute),
	}
	c.mu.Lock()
	c.repoCache[source] = entry
	c.mu.Unlock()
	return entry, true
}

func (c *Client) defaultBranch(source string) string {
	resp, err := c.httpClient.Get("https://api.github.com/repos/" + source)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	var r ghRepo
	if err := json.Unmarshal(body, &r); err != nil || r.DefaultBranch == "" {
		return "main"
	}
	return r.DefaultBranch
}

// parseFrontmatterName extracts the "name:" field from YAML frontmatter.
func parseFrontmatterName(content string) string {
	if !strings.HasPrefix(content, "---") {
		return ""
	}
	end := strings.Index(content[3:], "---")
	if end < 0 {
		return ""
	}
	frontmatter := content[3 : 3+end]
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			return strings.Trim(val, `"'`)
		}
	}
	return ""
}

func SanitizeSkillName(name string) string {
	name = strings.ToLower(name)
	replacer := strings.NewReplacer(":", "-", " ", "-", "/", "-", "\\", "-")
	name = replacer.Replace(name)
	var cleaned []byte
	for _, b := range []byte(name) {
		if (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '-' || b == '_' {
			cleaned = append(cleaned, b)
		}
	}
	return string(cleaned)
}

// BundleFile is one non-SKILL.md file that ships with a skill.
type BundleFile struct {
	// Path is relative to the skill directory: "scripts/deploy.sh".
	Path    string
	Content string
}

// maxBundleFiles and maxBundleBytes bound what one install will pull. Each file
// is a separate raw.githubusercontent request, so an unbounded skill directory
// is both slow and a good way to get rate-limited mid-install.
const (
	maxBundleFiles = 40
	maxBundleBytes = 2 * 1024 * 1024
)

// FetchSkillBundle returns the files that sit alongside a skill's SKILL.md.
//
// Installing used to fetch the single SKILL.md blob and nothing else, so a
// skill that ships scripts/ or references/ arrived gutted: the instructions
// still said "run scripts/deploy.sh" and the file was not there. The recursive
// tree call already lists every blob, so the siblings cost no extra lookup to
// find — only to download.
//
// Best-effort by design. A skill whose extras fail to download is still worth
// installing for its SKILL.md, so individual failures are skipped rather than
// failing the install.
func (c *Client) FetchSkillBundle(source, skillID string) ([]BundleFile, error) {
	skillPath, err := c.resolveSkillPath(source, skillID)
	if err != nil {
		return nil, err
	}
	dir := path.Dir(skillPath)

	repo, ok := c.repo(source)
	if !ok {
		return nil, fmt.Errorf("could not list %s", source)
	}

	var files []BundleFile
	var total int
	for _, blob := range repo.blobs {
		if blob == skillPath || !strings.HasPrefix(blob, dir+"/") {
			continue
		}
		if len(files) >= maxBundleFiles || total >= maxBundleBytes {
			break
		}
		content, ok := c.fetchAtPath(source, blob)
		if !ok {
			continue // binary or unreadable — skip, keep the rest
		}
		rel := strings.TrimPrefix(blob, dir+"/")
		files = append(files, BundleFile{Path: rel, Content: content})
		total += len(content)
	}
	return files, nil
}
