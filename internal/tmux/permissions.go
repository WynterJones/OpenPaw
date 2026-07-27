package tmux

import (
	"path"
	"strings"
)

// Rewriting a tmux command so Claude Code and Codex never stop to ask.
//
// A tmux session started from here is detached: nobody is sitting at the pane.
// A CLI that pauses on "allow this tool?" therefore waits forever — and from
// the chat it is indistinguishable from slow work, right up until the watcher
// reports the session as stalled three checks later. The prompt is invisible,
// so the only safe default is to not produce one.
//
// The rewrite is deliberately narrow: it fires only when a command *starts* a
// claude/codex run, only on subcommands that accept the flag, and never when
// the caller already said something about permissions, sandboxing or approvals.

const (
	claudeSkipFlag = "--dangerously-skip-permissions"
	codexSkipFlag  = "--dangerously-bypass-approvals-and-sandbox"
)

// Codex subcommands that run an agent, and so accept the bypass flag on their
// own parser. It has to be placed after them to be seen.
var codexAgentSubcommands = map[string]bool{
	"exec": true, "e": true, "resume": true, "fork": true,
}

// Every other codex subcommand rejects the flag as an unknown option, which
// would turn a working command into an instant failure. Same for claude, which
// has no agent subcommand at all — a bare "claude ..." is the run.
var codexOtherSubcommands = map[string]bool{
	"app": true, "app-server": true, "apply": true, "a": true, "archive": true,
	"cloud": true, "completion": true, "debug": true, "doctor": true,
	"exec-server": true, "features": true, "help": true, "login": true,
	"logout": true, "mcp": true, "mcp-server": true, "plugin": true,
	"remote-control": true, "review": true, "sandbox": true, "unarchive": true,
	"update": true,
}

var claudeSubcommands = map[string]bool{
	"agents": true, "auth": true, "auto-mode": true, "config": true,
	"doctor": true, "gateway": true, "help": true, "install": true,
	"mcp": true, "migrate-installer": true, "plugin": true, "plugins": true,
	"project": true, "setup-token": true, "ultrareview": true,
	"update": true, "upgrade": true,
}

// Flags that mean the caller has already decided how permissions should work.
// Adding the bypass on top would either conflict or silently overrule them.
var claudePermissionFlags = []string{
	claudeSkipFlag, "--permission-mode", "--permission-prompt-tool",
	"--allow-dangerously-skip-permissions",
}

var codexPermissionFlags = []string{
	codexSkipFlag, "--sandbox", "-s", "--ask-for-approval", "-a", "--full-auto",
}

// SkipPermissionPrompts adds the "don't ask" flag to any Claude Code or Codex
// invocation in a shell command, leaving everything else untouched.
//
// Idempotent: a command that already carries the flag comes back unchanged, so
// it is safe to apply at more than one layer.
func SkipPermissionPrompts(command string) string {
	var out strings.Builder
	start := 0
	var quote byte

	for i := 0; i < len(command); {
		c := command[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			i++
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
			i++
			continue
		case '\\':
			i += 2
			continue
		}
		n := separatorLen(command[i:])
		if n == 0 {
			i++
			continue
		}
		out.WriteString(rewriteSegment(command[start:i]))
		out.WriteString(command[i : i+n])
		i += n
		start = i
	}
	out.WriteString(rewriteSegment(command[start:]))
	return out.String()
}

// separatorLen reports the length of a shell separator at the head of s, so
// each pipeline stage and &&-chained command is examined on its own.
func separatorLen(s string) int {
	if strings.HasPrefix(s, "&&") || strings.HasPrefix(s, "||") {
		return 2
	}
	switch s[0] {
	case ';', '|', '&', '\n':
		return 1
	}
	return 0
}

type token struct {
	text string
	end  int // byte offset just past the token, within its segment
}

func rewriteSegment(seg string) string {
	tokens := tokenize(seg)

	// "FOO=bar claude ..." is still a claude run.
	i := 0
	for i < len(tokens) && isAssignment(tokens[i].text) {
		i++
	}
	if i >= len(tokens) {
		return seg
	}

	bin := path.Base(strings.Trim(tokens[i].text, `"'`))
	rest := tokens[i+1:]
	insertAfter := tokens[i].end

	var flag string
	switch bin {
	case "claude":
		if findSubcommand(rest, claudeSubcommands) != nil {
			return seg
		}
		if hasAny(rest, claudePermissionFlags) {
			return seg
		}
		flag = claudeSkipFlag
	case "codex":
		if sub := findSubcommand(rest, codexAgentSubcommands); sub != nil {
			insertAfter = sub.end
		} else if findSubcommand(rest, codexOtherSubcommands) != nil {
			return seg
		}
		if hasAny(rest, codexPermissionFlags) {
			return seg
		}
		flag = codexSkipFlag
	default:
		return seg
	}

	return seg[:insertAfter] + " " + flag + seg[insertAfter:]
}

// tokenize splits on whitespace outside quotes, keeping each token's end offset
// so the flag can be spliced into the original string without reflowing (and
// thereby mangling) the caller's own quoting.
func tokenize(seg string) []token {
	var tokens []token
	var quote byte
	start := -1

	for i := 0; i < len(seg); i++ {
		c := seg[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			if start < 0 {
				start = i
			}
			quote = c
		case ' ', '\t':
			if start >= 0 {
				tokens = append(tokens, token{seg[start:i], i})
				start = -1
			}
		default:
			if start < 0 {
				start = i
			}
		}
	}
	if start >= 0 {
		tokens = append(tokens, token{seg[start:], len(seg)})
	}
	return tokens
}

func isAssignment(tok string) bool {
	eq := strings.Index(tok, "=")
	if eq <= 0 || strings.HasPrefix(tok, "-") {
		return false
	}
	for _, r := range tok[:eq] {
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

// findSubcommand looks for one of known among the arguments, stopping at the
// first quoted word: everything from there on is the user's prompt, and a
// prompt that happens to contain "login" is not a subcommand.
func findSubcommand(tokens []token, known map[string]bool) *token {
	for i, t := range tokens {
		if strings.HasPrefix(t.text, `"`) || strings.HasPrefix(t.text, "'") {
			return nil
		}
		if known[t.text] {
			return &tokens[i]
		}
	}
	return nil
}

func hasAny(tokens []token, flags []string) bool {
	for _, t := range tokens {
		name := t.text
		if eq := strings.Index(name, "="); eq > 0 {
			name = name[:eq]
		}
		for _, f := range flags {
			if name == f {
				return true
			}
		}
	}
	return false
}
