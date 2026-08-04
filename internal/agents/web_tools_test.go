package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	llm "github.com/openpaw/openpaw/internal/llm"
)

// The reason this tool exists is that the agent's own shell cannot reach
// loopback, so a bare "localhost:3000" — what anyone would actually type — has
// to work.
func TestNormalizeURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"localhost:3000", "http://localhost:3000"},
		{"127.0.0.1:8080/health", "http://127.0.0.1:8080/health"},
		{"https://example.com/a", "https://example.com/a"},
		{" example.com ", "http://example.com"},
	}
	for _, c := range cases {
		got, err := normalizeURL(c.in)
		if err != nil {
			t.Errorf("normalizeURL(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("normalizeURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// A tool advertised as fetching web pages must not become a way to read
// arbitrary local files.
func TestNormalizeURL_RejectsNonHTTPSchemes(t *testing.T) {
	for _, in := range []string{"file:///etc/passwd", "ftp://host/x", "", "   "} {
		if _, err := normalizeURL(in); err == nil {
			t.Errorf("normalizeURL(%q) was accepted", in)
		}
	}
}

// The question fetch_url answers is "did the right content actually render",
// so script bodies and markup are noise that crowds out the answer.
func TestHTMLToText_KeepsWhatAReaderWouldSee(t *testing.T) {
	html := `<html><head><title>T</title><style>.a{color:red}</style></head>
	<body><script>var x = "not content";</script>
	<h1>Pricing</h1><p>From $29 &amp; up</p><!-- a note --><div>Cancel anytime</div></body></html>`

	got := htmlToText(html)

	for _, want := range []string{"Pricing", "From $29 & up", "Cancel anytime"} {
		if !strings.Contains(got, want) {
			t.Errorf("dropped visible content %q:\n%s", want, got)
		}
	}
	for _, banned := range []string{"not content", "color:red", "a note", "<h1>"} {
		if strings.Contains(got, banned) {
			t.Errorf("kept %q, which nobody sees on the page:\n%s", banned, got)
		}
	}
}

// Blocks have to break, or the whole page comes back as one unreadable line.
func TestHTMLToText_SeparatesBlocks(t *testing.T) {
	got := htmlToText("<p>one</p><p>two</p>")

	if !strings.Contains(got, "one\ntwo") {
		t.Errorf("blocks ran together: %q", got)
	}
}

func TestFetchURL_ReportsStatusAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusTeapot)
		w.Write([]byte("<html><body><h1>It is serving</h1></body></html>"))
	}))
	defer srv.Close()

	res := call(t, handleFetchURL, map[string]interface{}{"url": srv.URL})

	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	if !strings.Contains(res.Output, "HTTP 418") {
		t.Errorf("status is not reported:\n%s", res.Output)
	}
	if !strings.Contains(res.Output, "It is serving") {
		t.Errorf("body is not reported:\n%s", res.Output)
	}
}

// An unreachable local server is the common case, and the useful reply says
// where to look rather than just repeating the error.
func TestFetchURL_UnreachableSaysWhereToLook(t *testing.T) {
	res := call(t, handleFetchURL, map[string]interface{}{"url": "http://127.0.0.1:1/"})

	if !res.IsError {
		t.Fatal("expected an error for a port with nothing on it")
	}
	if !strings.Contains(res.Output, "tmux_list") {
		t.Errorf("the failure does not point at what is running:\n%s", res.Output)
	}
}

// max_chars protects the context window; without the cap a single page could
// fill it.
func TestFetchURL_TruncatesLongBodies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(strings.Repeat("x", 5000)))
	}))
	defer srv.Close()

	res := call(t, handleFetchURL, map[string]interface{}{"url": srv.URL, "max_chars": 100})

	if !strings.Contains(res.Output, "truncated at 100") {
		t.Errorf("long body was not truncated:\n%s", res.Output[:min(200, len(res.Output))])
	}
}

// Screenshots go where the agent can open them: a PNG saved somewhere its
// sandbox cannot read is a PNG nobody looks at.
func TestScreenshotURL_SavesInsideTheWorkingDirectory(t *testing.T) {
	work := t.TempDir()

	if got := shotBaseDir(work, "/data"); got != work {
		t.Errorf("shotBaseDir = %q, want the working directory %q", got, work)
	}
	if got := shotBaseDir("", "/data"); got != "/data" {
		t.Errorf("shotBaseDir with no workdir = %q, want the data dir", got)
	}
}

func TestBuildWebToolDefs_ShapeIsValid(t *testing.T) {
	defs := BuildWebToolDefs()
	handlers := MakeWebToolHandlers(t.TempDir())

	if len(defs) != 2 {
		t.Fatalf("got %d defs, want 2", len(defs))
	}
	for _, d := range defs {
		var schema map[string]interface{}
		if err := json.Unmarshal(d.Function.Parameters, &schema); err != nil {
			t.Errorf("%s: parameters are not valid JSON: %v", d.Function.Name, err)
		}
		if handlers[d.Function.Name] == nil {
			t.Errorf("%s is declared but has no handler", d.Function.Name)
		}
	}
}

// The habit being replaced is shipping a page having only read its source, so
// the tool has to say to open the image.
func TestScreenshotDef_TellsTheAgentToOpenTheFile(t *testing.T) {
	for _, d := range BuildWebToolDefs() {
		if d.Function.Name != "screenshot_url" {
			continue
		}
		if !strings.Contains(d.Function.Description, "Read") {
			t.Errorf("description does not say to open the saved image:\n%s", d.Function.Description)
		}
		return
	}
	t.Fatal("screenshot_url is missing")
}

func call(t *testing.T, h llm.ToolHandler, args map[string]interface{}) llm.ToolResult {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshalling args: %v", err)
	}
	return h(context.Background(), "", raw)
}

// Proof that the screenshot path works end to end, because the parts that can
// be wrong are all in the browser: current headless Chrome writes the image and
// then keeps running, so waiting for it to exit would time out on every
// successful shot. Skipped where no Chrome is installed.
func TestLive_ScreenshotWritesAnImage(t *testing.T) {
	if findBrowser() == "" {
		t.Skip("no Chrome-family browser installed")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body style="background:#111;color:#eee;font:48px sans-serif">Hello OpenPaw</body></html>`))
	}))
	defer srv.Close()

	work := t.TempDir()
	handler := handleScreenshotURL(work)

	raw, _ := json.Marshal(map[string]interface{}{"url": srv.URL, "width": 600, "height": 300, "wait_ms": 1500})
	res := handler(context.Background(), work, raw)

	if res.IsError {
		t.Fatalf("screenshot failed: %s", res.Output)
	}
	path := screenshotPath(t, res.Output)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the reported file does not exist: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("the screenshot is empty")
	}
	// It has to be somewhere the agent is allowed to read, or nobody opens it.
	if !strings.HasPrefix(path, work) {
		t.Errorf("screenshot saved outside the working directory: %s", path)
	}
	if !strings.Contains(res.Output, "Read") {
		t.Errorf("the reply does not tell the agent to open it:\n%s", res.Output)
	}
}

func screenshotPath(t *testing.T, output string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if strings.HasSuffix(strings.TrimSpace(line), ".png") {
			return strings.TrimSpace(line)
		}
	}
	t.Fatalf("no file path in the reply:\n%s", output)
	return ""
}
