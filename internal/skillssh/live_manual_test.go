package skillssh

import (
	"os"
	"strings"
	"testing"
)

// Hits skills.sh and GitHub for real. Opt-in via SKILLSSH_LIVE=1 so CI and the
// normal test run stay offline.
func TestLiveFetch(t *testing.T) {
	if os.Getenv("SKILLSSH_LIVE") == "" {
		t.Skip("set SKILLSSH_LIVE=1 to run")
	}
	c := NewClient()
	for _, name := range []string{"google-ads", "pdf", "product-hunt-launch"} {
		results, err := c.Search(name)
		if err != nil {
			t.Fatalf("search %s: %v", name, err)
		}
		want := SanitizeSkillName(name)
		found := false
		for _, r := range results {
			if SanitizeSkillName(r.SkillID) != want && SanitizeSkillName(r.Name) != want {
				continue
			}
			body, err := c.FetchSkillContent(r.Source, r.SkillID)
			if err != nil {
				t.Logf("  %s/%s -> ERROR %v", r.Source, r.SkillID, err)
				continue
			}
			found = true
			t.Logf("  %s/%s -> %d bytes, frontmatter name=%q", r.Source, r.SkillID, len(body), parseFrontmatterName(body))
			if len(body) < 200 {
				t.Errorf("%s/%s returned only %d bytes — looks like a stub", r.Source, r.SkillID, len(body))
			}
			if !strings.HasPrefix(body, "---") {
				t.Errorf("%s/%s has no frontmatter", r.Source, r.SkillID)
			}
		}
		if !found {
			t.Errorf("no exact match fetched for %q", name)
		}
	}
}
