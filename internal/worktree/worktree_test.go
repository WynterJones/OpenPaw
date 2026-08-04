package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlug(t *testing.T) {
	cases := []struct{ in, want string }{
		{"pace-site", "pace-site"},
		{"Fix RB Ads", "fix-rb-ads"},
		{"feature/checkout", "feature-checkout"},
		{"  ", "task"},
		{"!!!", "task"},
		{strings.Repeat("a", 60), strings.Repeat("a", 40)},
	}
	for _, c := range cases {
		if got := Slug(c.in); got != c.want {
			t.Errorf("Slug(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Isolation that silently didn't happen is worse than none, because the caller
// proceeds as though two sessions cannot collide.
func TestCreate_RefusesOutsideARepository(t *testing.T) {
	requireGit(t)

	if _, err := Create(context.Background(), t.TempDir(), "task"); err == nil {
		t.Fatal("expected an error for a directory that is not a git repository")
	}
}

func TestCreate_GivesEachSessionItsOwnTreeAndBranch(t *testing.T) {
	requireGit(t)
	repo := newRepo(t)

	first, err := Create(context.Background(), repo, "pace-site")
	if err != nil {
		t.Fatalf("first worktree: %v", err)
	}
	second, err := Create(context.Background(), repo, "pace-site")
	if err != nil {
		t.Fatalf("second worktree: %v", err)
	}

	if first.Path == second.Path {
		t.Error("both sessions were given the same directory, which is the collision this prevents")
	}
	if first.Branch == second.Branch {
		t.Error("both sessions were given the same branch")
	}
	if !strings.HasPrefix(first.Branch, branchPrefix) {
		t.Errorf("branch %q is not namespaced, so nobody can tell it was machine-made", first.Branch)
	}

	// Sibling, never nested: a worktree inside the repository turns up in its
	// own status and build globs.
	if strings.HasPrefix(first.Path, repo+string(filepath.Separator)) {
		t.Errorf("worktree %q is inside the repository %q", first.Path, repo)
	}
	if _, err := os.Stat(filepath.Join(first.Path, "README.md")); err != nil {
		t.Errorf("the worktree has no checkout in it: %v", err)
	}
}

// The list is what an agent uses to find where its dispatched work landed.
func TestList_ReportsOnlyOurWorktrees(t *testing.T) {
	requireGit(t)
	repo := newRepo(t)

	made, err := Create(context.Background(), repo, "audit")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	trees, err := List(context.Background(), repo)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(trees) != 1 {
		t.Fatalf("got %d worktrees, want 1 (the main checkout must not be listed)", len(trees))
	}
	if trees[0].Branch != made.Branch {
		t.Errorf("branch = %q, want %q", trees[0].Branch, made.Branch)
	}
}

// Removal is the cleanup step that once took ~1,950 lines of uncommitted work
// with it, so refusing is the correct behaviour when anything is unsaved.
func TestRemove_RefusesUncommittedWork(t *testing.T) {
	requireGit(t)
	repo := newRepo(t)

	made, err := Create(context.Background(), repo, "risky")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := os.WriteFile(filepath.Join(made.Path, "notes.md"), []byte("unsaved work"), 0644); err != nil {
		t.Fatalf("writing a file: %v", err)
	}
	run(t, made.Path, "git", "add", "notes.md")

	if err := Remove(context.Background(), made.Path); err == nil {
		t.Fatal("removed a worktree holding uncommitted work")
	}
	if _, err := os.Stat(made.Path); err != nil {
		t.Errorf("the worktree was deleted anyway: %v", err)
	}
}

func TestRemove_RefusesPathsItDidNotCreate(t *testing.T) {
	requireGit(t)
	repo := newRepo(t)

	if err := Remove(context.Background(), repo); err == nil {
		t.Fatal("removed a path that is not an OpenPaw worktree")
	}
}

func TestRemove_TakesTheBranchWithIt(t *testing.T) {
	requireGit(t)
	repo := newRepo(t)

	made, err := Create(context.Background(), repo, "clean")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := Remove(context.Background(), made.Path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(made.Path); !os.IsNotExist(err) {
		t.Error("the worktree directory is still there")
	}
	if branchExists(context.Background(), repo, made.Branch) {
		t.Errorf("branch %q outlived its worktree", made.Branch)
	}
}

func requireGit(t *testing.T) {
	t.Helper()
	if !Available() {
		t.Skip("git is not installed")
	}
}

// newRepo builds a one-commit repository inside a temp directory, along with
// the parent the worktrees will be created next to.
func newRepo(t *testing.T) string {
	t.Helper()

	parent := t.TempDir()
	repo := filepath.Join(parent, "app")
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	run(t, repo, "git", "init", "-b", "main")
	run(t, repo, "git", "config", "user.email", "test@example.com")
	run(t, repo, "git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0644); err != nil {
		t.Fatalf("writing README: %v", err)
	}
	run(t, repo, "git", "add", ".")
	run(t, repo, "git", "commit", "-m", "first")

	// git worktree resolves paths through their real location, and a macOS temp
	// directory is reached through a symlink, so the created path would not
	// match the one this package computed.
	real, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatalf("resolving %s: %v", repo, err)
	}
	return real
}

func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}
