package tmux

import (
	"os"
	"strings"
	"testing"
)

// The whole point is that the value never reaches a command line, where ps
// would publish it to every process on the machine.
func TestWithEnvFile_KeepsValuesOffTheCommandLine(t *testing.T) {
	secret := "sk-live-do-not-print"

	command, err := withEnvFile("railway up", map[string]string{"RAILWAY_TOKEN": secret})
	if err != nil {
		t.Fatalf("withEnvFile: %v", err)
	}

	if strings.Contains(command, secret) {
		t.Errorf("the value is in the command itself:\n%s", command)
	}
	if !strings.HasSuffix(command, "railway up") {
		t.Errorf("the original command is not what runs:\n%s", command)
	}
	if !strings.Contains(command, "rm -f") {
		t.Errorf("the environment file is never deleted:\n%s", command)
	}

	path := envFilePath(t, command)
	t.Cleanup(func() { os.Remove(path) })

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the env file: %v", err)
	}
	if want := "export RAILWAY_TOKEN='" + secret + "'\n"; string(body) != want {
		t.Errorf("env file = %q, want %q", body, want)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("env file mode = %04o, want 0600 — anyone on the machine can read it", perm)
	}
}

// A value with a quote in it must not be able to end the quoting and become
// shell syntax; a secret is arbitrary bytes, not an identifier.
func TestShellQuote_ContainsAnyValue(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", `'plain'`},
		{"has space", `'has space'`},
		{"it's", `'it'\''s'`},
		{"; rm -rf /", `'; rm -rf /'`},
		{"$(whoami)", `'$(whoami)'`},
		{"`id`", "'`id`'"},
	}
	for _, c := range cases {
		if got := shellQuote(c.in); got != c.want {
			t.Errorf("shellQuote(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}

// envFilePath pulls the temp file out of the generated command: ". '<path>'; …".
func envFilePath(t *testing.T, command string) string {
	t.Helper()
	open := strings.Index(command, "'")
	close := strings.Index(command[open+1:], "'")
	if open < 0 || close < 0 {
		t.Fatalf("no quoted path in %q", command)
	}
	return command[open+1 : open+1+close]
}
