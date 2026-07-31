package handlers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePathByNameStaysInCommonFolders(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	downloads := filepath.Join(home, "Downloads")
	if err := os.MkdirAll(downloads, 0o755); err != nil {
		t.Fatal(err)
	}
	wanted := filepath.Join(downloads, "project")
	if err := os.Mkdir(wanted, 0o755); err != nil {
		t.Fatal(err)
	}

	protected := filepath.Join(home, "Library", "Containers", "other.app", "project")
	if err := os.MkdirAll(protected, 0o755); err != nil {
		t.Fatal(err)
	}

	if got := resolvePathByName("project", true); got != wanted {
		t.Fatalf("resolvePathByName() = %q, want %q", got, wanted)
	}

	if err := os.RemoveAll(wanted); err != nil {
		t.Fatal(err)
	}
	if got := resolvePathByName("project", true); got != "" {
		t.Fatalf("resolved protected app data outside common folders: %q", got)
	}
}

func TestResolvePathByNameRejectsPathTraversal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if got := resolvePathByName("../Library", true); got != "" {
		t.Fatalf("resolvePathByName() accepted traversal and returned %q", got)
	}
}
