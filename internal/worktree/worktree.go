// Package worktree gives a dispatched agent its own checkout of a repository.
//
// Two agents in one working directory are not isolated, whatever else keeps
// them apart. They share a branch, an index and a set of uncommitted files, so
// one moving the branch under the other, or a cleanup step taking the other's
// unstaged work with it, is not a mistake anyone made — it is what a shared
// directory means. tmux sessions look like isolation and aren't: separate
// terminals, same tree.
//
// A worktree is the real thing. Each session gets its own directory and its own
// branch off the same commit, and git keeps one object store underneath, so the
// cost is a checkout rather than a clone.
package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// dirName is the sibling directory worktrees are created under. Sibling rather
// than nested: a worktree inside the repository shows up in its own status, in
// test globs and in build inputs, so the isolation would leak straight back
// into the thing it was meant to protect.
const dirName = ".openpaw-worktrees"

// branchPrefix namespaces the branches this package creates so they are
// recognisable as machine-made and safe to delete.
const branchPrefix = "openpaw/"

// Info describes one worktree created for a session.
type Info struct {
	Path   string `json:"path"`
	Branch string `json:"branch"`
	Repo   string `json:"repo"`
	Base   string `json:"base"`
}

// Available reports whether git is installed.
func Available() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// Create makes a worktree for name, branched from the repository's current
// HEAD, and returns where it landed.
func Create(ctx context.Context, repoDir, name string) (Info, error) {
	if !Available() {
		return Info{}, errors.New("git is not installed")
	}
	root, err := Root(ctx, repoDir)
	if err != nil {
		return Info{}, err
	}

	slug := Slug(name)
	base := baseRef(ctx, root)

	parent := filepath.Join(filepath.Dir(root), dirName, filepath.Base(root))
	if err := os.MkdirAll(parent, 0755); err != nil {
		return Info{}, fmt.Errorf("could not create %s: %w", parent, err)
	}

	path, branch := freeNames(ctx, root, parent, slug)

	// -b creates the branch as part of the checkout, so the worktree starts on
	// a branch of its own rather than a detached HEAD that a later commit would
	// strand.
	if out, err := git(ctx, root, "worktree", "add", "-b", branch, path, base); err != nil {
		return Info{}, fmt.Errorf("git worktree add failed: %s", firstLine(out, err))
	}
	return Info{Path: path, Branch: branch, Repo: root, Base: base}, nil
}

// Remove deletes a worktree and the branch created with it.
//
// The branch goes too because it is only useful attached to the tree: what the
// session produced is either merged by now or being abandoned, and a repository
// slowly filling with openpaw/* branches nobody can place is its own mess. An
// unmerged branch is kept — losing work is the failure this package exists to
// prevent, so it refuses rather than forcing.
func Remove(ctx context.Context, path string) error {
	if !Available() {
		return errors.New("git is not installed")
	}
	path = filepath.Clean(path)
	if !strings.Contains(path, dirName) {
		return fmt.Errorf("%s is not an OpenPaw worktree, so it will not be removed", path)
	}
	root, err := Root(ctx, path)
	if err != nil {
		return err
	}
	branch := currentBranch(ctx, path)

	if out, err := git(ctx, root, "worktree", "remove", path); err != nil {
		return fmt.Errorf("git worktree remove failed (commit or discard the changes in it first): %s", firstLine(out, err))
	}
	if strings.HasPrefix(branch, branchPrefix) {
		// -d, never -D: an unmerged branch stays, and the caller is told.
		if out, delErr := git(ctx, root, "branch", "-d", branch); delErr != nil {
			return fmt.Errorf("removed the worktree, but kept branch %s because it has unmerged commits: %s",
				branch, firstLine(out, delErr))
		}
	}
	return nil
}

// List returns the worktrees this package created for a repository.
func List(ctx context.Context, repoDir string) ([]Info, error) {
	if !Available() {
		return nil, errors.New("git is not installed")
	}
	root, err := Root(ctx, repoDir)
	if err != nil {
		return nil, err
	}
	out, err := git(ctx, root, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git worktree list failed: %s", firstLine(out, err))
	}

	var infos []Info
	var current Info
	flush := func() {
		if current.Path != "" && strings.Contains(current.Path, dirName) {
			current.Repo = root
			infos = append(infos, current)
		}
		current = Info{}
	}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			current.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "branch "):
			current.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		}
	}
	flush()
	return infos, nil
}

// Root returns the main working tree of the repository containing dir — the
// original checkout, not whichever worktree dir happens to sit in.
//
// The distinction is not academic: --show-toplevel inside a worktree returns
// that worktree, so removing one from inside itself left every follow-up git
// command trying to chdir into a directory that had just been deleted. Every
// worktree shares one common git dir, and its parent is the main tree.
func Root(ctx context.Context, dir string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", errors.New("no directory to work in")
	}
	top, err := git(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("%s is not inside a git repository, so there is nothing to isolate", dir)
	}
	top = strings.TrimSpace(top)

	common, err := git(ctx, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		// --path-format needs git 2.31; without it the common dir may come back
		// relative to dir, which resolves to the same place.
		if common, err = git(ctx, dir, "rev-parse", "--git-common-dir"); err != nil {
			return top, nil
		}
	}
	path := strings.TrimSpace(common)
	if path == "" {
		return top, nil
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	// A bare repository has no working tree above its git dir to fall back to.
	if root := filepath.Dir(filepath.Clean(path)); root != "" && root != "." {
		if _, statErr := os.Stat(root); statErr == nil {
			return root, nil
		}
	}
	return top, nil
}

// Slug turns a session label into something usable as both a directory and a
// branch name.
func Slug(name string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(strings.ToLower(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ', r == '.', r == '/', r == ':':
			b.WriteRune('-')
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "task"
	}
	if len(slug) > 40 {
		slug = strings.Trim(slug[:40], "-")
	}
	return slug
}

// freeNames picks a directory and branch that are both free, so dispatching the
// same task twice does not fail on a name the first one is still holding.
func freeNames(ctx context.Context, root, parent, slug string) (path, branch string) {
	for i := 0; i < 100; i++ {
		candidate := slug
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", slug, i+1)
		}
		path = filepath.Join(parent, candidate)
		branch = branchPrefix + candidate
		if !exists(path) && !branchExists(ctx, root, branch) {
			return path, branch
		}
	}
	// Nothing sensible left to try; a timestamp is unique enough.
	stamp := fmt.Sprintf("%s-%d", slug, time.Now().Unix())
	return filepath.Join(parent, stamp), branchPrefix + stamp
}

// baseRef is the commit new worktrees branch from: the repository's current
// HEAD, so a session starts from what the user is actually looking at. A
// repository with no commits yet has no HEAD to name.
func baseRef(ctx context.Context, root string) string {
	if out, err := git(ctx, root, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		if ref := strings.TrimSpace(out); ref != "" && ref != "HEAD" {
			return ref
		}
	}
	return "HEAD"
}

func currentBranch(ctx context.Context, dir string) string {
	out, err := git(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func branchExists(ctx context.Context, root, branch string) bool {
	_, err := git(ctx, root, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// firstLine keeps git's own explanation, which is nearly always more use than
// the exit status it came with.
func firstLine(out string, err error) string {
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if s := strings.TrimSpace(line); s != "" {
			return s
		}
	}
	return err.Error()
}
