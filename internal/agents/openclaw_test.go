package agents

import "testing"

func TestParseOpenClawReply(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"result payloads",
			`{"ok":true,"result":{"payloads":[{"text":"Hello from Nabu"}]}}`,
			"Hello from Nabu",
		},
		{
			"top-level payloads multi",
			`{"payloads":[{"text":"part one"},{"text":"part two"}]}`,
			"part one\n\npart two",
		},
		{
			"reply field",
			`{"reply":"direct reply"}`,
			"direct reply",
		},
		{
			"result text field",
			`{"result":{"text":"inner text"}}`,
			"inner text",
		},
		{
			"log noise before json",
			"warn: something\n{\"reply\":\"after noise\"}",
			"after noise",
		},
		{
			"empty",
			``,
			"",
		},
		{
			"no text anywhere",
			`{"ok":true,"status":"delivered"}`,
			"",
		},
	}
	for _, c := range cases {
		if got := parseOpenClawReply([]byte(c.in)); got != c.want {
			t.Errorf("%s: parseOpenClawReply = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestExtractCLIJSON(t *testing.T) {
	if got := string(extractCLIJSON([]byte("log line\n[{\"id\":\"main\"}]\n"))); got != `[{"id":"main"}]` {
		t.Errorf("array extraction = %q", got)
	}
	if got := extractCLIJSON([]byte("no json here")); got != nil {
		t.Errorf("expected nil for non-JSON, got %q", got)
	}
}
