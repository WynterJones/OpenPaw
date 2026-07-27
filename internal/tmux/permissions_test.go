package tmux

import "testing"

func TestSkipPermissionPrompts(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{
			"bare claude",
			`claude`,
			`claude --dangerously-skip-permissions`,
		},
		{
			"claude with a prompt",
			`claude -p "fix the failing tests"`,
			`claude --dangerously-skip-permissions -p "fix the failing tests"`,
		},
		{
			"absolute path to claude",
			`/Users/w/.local/bin/claude "go"`,
			`/Users/w/.local/bin/claude --dangerously-skip-permissions "go"`,
		},
		{
			"claude after a cd",
			`cd /tmp/x && claude -p "build it"`,
			`cd /tmp/x && claude --dangerously-skip-permissions -p "build it"`,
		},
		{
			"env assignment before claude",
			`FOO=bar claude -p "go"`,
			`FOO=bar claude --dangerously-skip-permissions -p "go"`,
		},
		{
			"bare codex",
			`codex`,
			`codex --dangerously-bypass-approvals-and-sandbox`,
		},
		{
			"codex with a prompt",
			`codex "refactor the parser"`,
			`codex --dangerously-bypass-approvals-and-sandbox "refactor the parser"`,
		},
		{
			// The flag has to land after the subcommand: exec parses its own.
			"codex exec",
			`codex exec --json "run the suite"`,
			`codex exec --dangerously-bypass-approvals-and-sandbox --json "run the suite"`,
		},
		{
			"codex resume",
			`codex resume --last`,
			`codex resume --dangerously-bypass-approvals-and-sandbox --last`,
		},
		{
			"both sides of a pipe",
			`claude -p "x" | codex exec -`,
			`claude --dangerously-skip-permissions -p "x" | codex exec --dangerously-bypass-approvals-and-sandbox -`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SkipPermissionPrompts(tt.command); got != tt.want {
				t.Errorf("SkipPermissionPrompts(%q)\n got: %q\nwant: %q", tt.command, got, tt.want)
			}
		})
	}
}

// Commands that must come back byte-for-byte unchanged: either they are not an
// agent run, or the caller already decided how permissions should work.
func TestSkipPermissionPromptsLeavesAlone(t *testing.T) {
	unchanged := []string{
		`npm run build`,
		`go test ./...`,
		`echo claude`,
		`npm run claude-thing`,
		`claude mcp list`,
		`claude update`,
		`claude doctor`,
		`codex login`,
		`codex mcp list`,
		`codex apply`,
		`claude --dangerously-skip-permissions -p "x"`,
		`claude --permission-mode acceptEdits`,
		`codex exec --sandbox workspace-write "x"`,
		`codex --ask-for-approval never "x"`,
		`git commit -m "run claude later"`,
	}
	for _, cmd := range unchanged {
		t.Run(cmd, func(t *testing.T) {
			if got := SkipPermissionPrompts(cmd); got != cmd {
				t.Errorf("SkipPermissionPrompts(%q) = %q, want it unchanged", cmd, got)
			}
		})
	}
}

// Applying the rewrite twice must not add the flag twice — Start applies it as
// a backstop even when the caller already has.
func TestSkipPermissionPromptsIdempotent(t *testing.T) {
	for _, cmd := range []string{`claude -p "x"`, `codex exec "y"`, `codex`} {
		once := SkipPermissionPrompts(cmd)
		if twice := SkipPermissionPrompts(once); twice != once {
			t.Errorf("second pass over %q changed it:\n once: %q\ntwice: %q", cmd, once, twice)
		}
	}
}
