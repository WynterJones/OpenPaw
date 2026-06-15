package platform

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// FixPath augments the process PATH so CLI tools installed in the user's shell
// environment (claude, codex, openclaw, node, etc.) are discoverable via
// exec.LookPath.
//
// GUI applications on macOS (and to a lesser extent Linux) launched from
// Finder/Dock/launchd inherit a minimal PATH — typically only
// /usr/bin:/bin:/usr/sbin:/sbin — rather than the rich PATH a login shell
// builds from ~/.zshrc, nvm, asdf, homebrew, ~/.local/bin and friends. When
// OpenPaw runs as a Tauri sidecar this means none of the CLI providers are
// found and they all show as "not installed".
//
// FixPath first asks the user's login shell for its PATH (the authoritative
// source) and, regardless of whether that succeeds, prepends a set of common
// install directories. It is safe to call multiple times and is a no-op on
// Windows, where GUI apps already inherit the system PATH.
func FixPath() {
	if runtime.GOOS == "windows" {
		return
	}

	existing := splitPath(os.Getenv("PATH"))
	seen := make(map[string]bool, len(existing))
	for _, p := range existing {
		seen[p] = true
	}

	var ordered []string
	add := func(dirs []string) {
		for _, d := range dirs {
			if d == "" || seen[d] {
				continue
			}
			seen[d] = true
			ordered = append(ordered, d)
		}
	}

	// 1. The login shell's PATH is the most accurate source.
	add(loginShellPath())

	// 2. Well-known locations as a fallback (shell resolution can fail in
	//    sandboxed/headless contexts, and a fresh shell may not export
	//    everything an interactive one does).
	add(commonBinDirs())

	if len(ordered) == 0 {
		return
	}

	// Existing entries keep priority; newly discovered dirs are appended so we
	// never shadow a tool the process could already find.
	merged := append(existing, ordered...)
	os.Setenv("PATH", strings.Join(merged, string(os.PathListSeparator)))
}

// loginShellPath runs the user's login shell as an interactive login shell and
// captures the PATH it exports. Returns nil if the shell can't be resolved or
// the command fails.
func loginShellPath() []string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// -i -l -c runs an interactive login shell so rc files (which set up nvm,
	// asdf, homebrew shellenv, etc.) are sourced. We delimit the value so any
	// banner output printed by rc files can be stripped reliably.
	cmd := exec.CommandContext(ctx, shell, "-ilc", `printf "__OPENPAW_PATH__%s__END__" "$PATH"`)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	s := string(out)
	start := strings.Index(s, "__OPENPAW_PATH__")
	end := strings.Index(s, "__END__")
	if start < 0 || end < 0 || end < start {
		return nil
	}
	value := s[start+len("__OPENPAW_PATH__") : end]
	return splitPath(value)
}

// commonBinDirs returns CLI install locations that are frequently missing from
// a GUI app's inherited PATH.
func commonBinDirs() []string {
	dirs := []string{
		"/opt/homebrew/bin", // Apple Silicon homebrew
		"/usr/local/bin",    // Intel homebrew / common installs
		"/usr/bin",
		"/bin",
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs,
			filepath.Join(home, ".local", "bin"), // pipx, claude installer
			filepath.Join(home, "bin"),
			filepath.Join(home, ".cargo", "bin"),      // rust
			filepath.Join(home, ".npm-global", "bin"), // npm global prefix
			filepath.Join(home, ".volta", "bin"),      // volta
			filepath.Join(home, ".asdf", "shims"),     // asdf
			filepath.Join(home, "go", "bin"),          // go install
		)
	}
	return dirs
}

func splitPath(p string) []string {
	if p == "" {
		return nil
	}
	parts := strings.Split(p, string(os.PathListSeparator))
	out := parts[:0]
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
