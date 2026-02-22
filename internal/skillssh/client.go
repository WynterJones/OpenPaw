package skillssh

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

type Client struct {
	httpClient *http.Client
	mu         sync.RWMutex
	cache      map[string]cacheEntry
	dirCache   map[string]string // maps "source/skillName" -> directory name
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		cache:      make(map[string]cacheEntry),
		dirCache:   make(map[string]string),
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

func (c *Client) FetchSkillContent(source, skillID string) (string, error) {
	// Check directory name cache first.
	cacheKey := source + "/" + skillID
	c.mu.RLock()
	dirName, cached := c.dirCache[cacheKey]
	c.mu.RUnlock()

	if cached {
		rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/main/skills/%s/SKILL.md", source, dirName)
		content, status, err := c.fetchRaw(rawURL)
		if err != nil {
			return "", fmt.Errorf("fetch skill: %w", err)
		}
		if status == http.StatusOK {
			return content, nil
		}
	}

	// Try skillID as-is (it may match the directory name directly).
	rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/main/skills/%s/SKILL.md", source, skillID)
	content, status, err := c.fetchRaw(rawURL)
	if err != nil {
		return "", fmt.Errorf("fetch skill: %w", err)
	}
	if status == http.StatusOK {
		c.mu.Lock()
		c.dirCache[cacheKey] = skillID
		c.mu.Unlock()
		return content, nil
	}

	// Direct path didn't work. The skillID is the frontmatter name, not the
	// directory name. List the skills/ directory via GitHub API and find the
	// directory whose SKILL.md has a matching name in its frontmatter.
	resolved, err := c.resolveSkillDir(source, skillID)
	if err != nil {
		return "", fmt.Errorf("resolve skill directory for %s/%s: %w", source, skillID, err)
	}

	rawURL = fmt.Sprintf("https://raw.githubusercontent.com/%s/main/skills/%s/SKILL.md", source, resolved)
	content, status, err = c.fetchRaw(rawURL)
	if err != nil {
		return "", fmt.Errorf("fetch skill: %w", err)
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("GitHub raw returned %d for %s/skills/%s", status, source, resolved)
	}

	c.mu.Lock()
	c.dirCache[cacheKey] = resolved
	c.mu.Unlock()

	return content, nil
}

type ghContentEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// resolveSkillDir lists the skills/ directory in the GitHub repo and checks
// each subdirectory's SKILL.md frontmatter to find the one whose name matches
// the given skillID.
func (c *Client) resolveSkillDir(source, skillID string) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/contents/skills", source)
	resp, err := c.httpClient.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("list skills directory: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %d listing skills/", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read directory listing: %w", err)
	}

	var entries []ghContentEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return "", fmt.Errorf("parse directory listing: %w", err)
	}

	// Check each subdirectory's SKILL.md for a matching name.
	for _, entry := range entries {
		if entry.Type != "dir" {
			continue
		}

		rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/main/skills/%s/SKILL.md", source, entry.Name)
		content, status, err := c.fetchRaw(rawURL)
		if err != nil || status != http.StatusOK {
			continue
		}

		name := parseFrontmatterName(content)
		if name == "" {
			continue
		}

		// Cache this mapping for future lookups.
		key := source + "/" + name
		c.mu.Lock()
		c.dirCache[key] = entry.Name
		c.mu.Unlock()

		if name == skillID {
			return entry.Name, nil
		}
	}

	return "", fmt.Errorf("skill %q not found in %s", skillID, source)
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
