package tmux

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// envFileLifetime is how long the file is left on disk as a backstop. The
// command sources and deletes it within a second of starting; this only covers
// a session that failed to start at all, so nothing is left lying around
// holding a credential.
const envFileLifetime = 2 * time.Minute

// withEnvFile writes env to a private file and returns a command that sources
// it, removes it, and then runs the original command with those variables set.
//
// Written to a file rather than exported inline because a tmux command line is
// world-readable through ps: `FOO=secret npm run deploy` publishes the secret
// to every process on the machine for as long as it runs. The file is 0600 and
// unlinked by the command itself before any of the real work starts, so the
// window in which it exists is a fraction of a second.
func withEnvFile(command string, env map[string]string) (string, error) {
	f, err := os.CreateTemp("", "openpaw-env-*.sh")
	if err != nil {
		return "", fmt.Errorf("could not create the environment file: %w", err)
	}
	path := f.Name()

	// Written before the values go in: CreateTemp already makes the file 0600,
	// but being explicit means a changed default cannot quietly widen it.
	if err := f.Chmod(0600); err != nil {
		f.Close()
		os.Remove(path)
		return "", fmt.Errorf("could not secure the environment file: %w", err)
	}

	// Sorted so the file is deterministic, which makes it testable.
	names := make([]string, 0, len(env))
	for name := range env {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		fmt.Fprintf(&b, "export %s=%s\n", name, shellQuote(env[name]))
	}
	if _, err := f.WriteString(b.String()); err != nil {
		f.Close()
		os.Remove(path)
		return "", fmt.Errorf("could not write the environment file: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("could not write the environment file: %w", err)
	}

	time.AfterFunc(envFileLifetime, func() { os.Remove(path) })

	return fmt.Sprintf(". %s; rm -f %s; %s",
		shellQuote(path), shellQuote(path), command), nil
}

// shellQuote wraps s in single quotes for /bin/sh, which is the only quoting
// that is safe for an arbitrary secret: inside single quotes every character
// is literal, and the one that cannot appear is escaped by closing the quote
// around it.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
