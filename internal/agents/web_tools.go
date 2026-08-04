package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	llm "github.com/openpaw/openpaw/internal/llm"
)

// Looking at what was just built.
//
// A CLI agent runs inside a sandbox whose network goes through a proxy, and the
// proxy does not carry loopback: curl to 127.0.0.1 comes back 000 whatever is
// listening. So an agent could start a dev server, build a page, deploy a site
// — and then had to ask the user to go and look at it, or settle for grepping
// the HTML it had just written, which only ever confirms its own assumptions.
//
// These two tools run in the OpenPaw server process, which is not sandboxed.
// fetch_url answers "is it actually serving, and what does it say"; screenshot
// answers the question grep cannot: what does it look like. The screenshot is
// written into the agent's own working directory precisely so it can be opened
// with Read — an image an agent cannot open is no better than the URL was.

const (
	fetchTimeout      = 30 * time.Second
	fetchMaxBytes     = 4 << 20 // 4 MB read ceiling; pages beyond this are not worth reading
	fetchDefaultChars = 15000
	fetchMaxChars     = 60000
	screenshotTimeout = 90 * time.Second
)

// BuildWebToolDefs returns the tools for looking at a running page.
func BuildWebToolDefs() []llm.ToolDef {
	fetchParams, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type": "string",
				"description": "The URL to fetch. Localhost and 127.0.0.1 work — this runs outside your " +
					"sandbox, so a local dev server is reachable.",
			},
			"max_chars": map[string]interface{}{
				"type":        "integer",
				"description": "How much of the body to return. Defaults to 15000, capped at 60000.",
				"default":     fetchDefaultChars,
			},
			"raw": map[string]interface{}{
				"type": "boolean",
				"description": "Return the HTML source instead of the readable text. Use it when you need " +
					"to check markup, a meta tag or an asset path.",
				"default": false,
			},
		},
		"required": []string{"url"},
	})

	shotParams, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "The URL to photograph. Localhost works.",
			},
			"width": map[string]interface{}{
				"type":        "integer",
				"description": "Viewport width in pixels. Defaults to 1440. Use 390 to check a phone layout.",
				"default":     1440,
			},
			"height": map[string]interface{}{
				"type":        "integer",
				"description": "Viewport height in pixels. Defaults to 900.",
				"default":     900,
			},
			"wait_ms": map[string]interface{}{
				"type": "integer",
				"description": "How long to let the page render before the shot, in milliseconds. " +
					"Defaults to 4000. Raise it for a page that fetches its content.",
				"default": 4000,
			},
		},
		"required": []string{"url"},
	})

	return []llm.ToolDef{
		{
			Type: "function",
			Function: llm.FunctionDef{
				Name: "fetch_url",
				Description: "Fetch a URL from the OpenPaw server, which is outside your sandbox — so " +
					"http://localhost:3000 and 127.0.0.1 are reachable when they are not from your own shell. " +
					"Returns the status, the content type and the page's readable text. USE THIS to check " +
					"that a dev server is actually serving what you think, to read a page you just deployed, " +
					"or to call a local API.",
				Parameters: fetchParams,
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDef{
				Name: "screenshot_url",
				Description: "Photograph a page in a headless browser and save it as a PNG, then OPEN THAT " +
					"FILE WITH Read — you can see images. USE THIS before telling anyone a page is finished. " +
					"Reading the HTML tells you what you wrote; the screenshot tells you what it looks like, " +
					"which is where broken layout, invisible text and a collapsed hero actually show up. " +
					"Also use it to check a phone width.",
				Parameters: shotParams,
			},
		},
	}
}

// MakeWebToolHandlers returns the handlers. dataDir is the fallback location
// for screenshots when a run has no working directory of its own.
func MakeWebToolHandlers(dataDir string) map[string]llm.ToolHandler {
	return map[string]llm.ToolHandler{
		"fetch_url":      handleFetchURL,
		"screenshot_url": handleScreenshotURL(dataDir),
	}
}

// buildWebPromptSection tells the agent to go and look, because the default
// behaviour without it is to ship a page having only ever read its source.
func buildWebPromptSection() string {
	return `## LOOK AT WHAT YOU BUILD

Your own shell cannot reach loopback — ` + "`curl http://127.0.0.1:3000`" + ` returns nothing whatever is listening there. OpenPaw fetches for you instead, from outside that sandbox.

- ` + "`fetch_url`" + ` — read a page or call an API, including ` + "`localhost`" + `. Use it to confirm a dev server is serving what you think it is.
- ` + "`screenshot_url`" + ` — photograph a page, then **open the saved PNG with ` + "`Read`" + `**. You can see images.

**Before you tell anyone a page is done, look at it.** Reading the HTML you just wrote confirms your own assumptions and nothing else; a broken layout, text the same colour as its background, a hero that collapsed on mobile and a component that never mounted all look fine in the source. Take the screenshot, open it, and say what you saw.`
}

func handleFetchURL(ctx context.Context, _ string, input json.RawMessage) llm.ToolResult {
	var req struct {
		URL      string `json:"url"`
		MaxChars int    `json:"max_chars"`
		Raw      bool   `json:"raw"`
	}
	json.Unmarshal(input, &req)

	target, err := normalizeURL(req.URL)
	if err != nil {
		return llm.ToolResult{Output: err.Error(), IsError: true}
	}

	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return llm.ToolResult{Output: "Could not build the request: " + err.Error(), IsError: true}
	}
	// Some dev servers and CDNs serve a different page, or nothing at all, to a
	// client that does not identify itself as a browser.
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (compatible; OpenPaw/1.0; +https://openpaw.app)")
	httpReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/json;q=0.9,*/*;q=0.8")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return llm.ToolResult{Output: fmt.Sprintf(
			"Could not reach %s: %v\n\nIf this is a local server, check it is running — `tmux_list` shows "+
				"what OpenPaw has started.", target, err), IsError: true}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, fetchMaxBytes))
	if err != nil {
		return llm.ToolResult{Output: "Could not read the response: " + err.Error(), IsError: true}
	}

	contentType := resp.Header.Get("Content-Type")
	text := string(body)
	if !req.Raw && strings.Contains(contentType, "html") {
		text = htmlToText(text)
	}

	limit := req.MaxChars
	switch {
	case limit <= 0:
		limit = fetchDefaultChars
	case limit > fetchMaxChars:
		limit = fetchMaxChars
	}
	truncated := ""
	if len(text) > limit {
		text = text[:limit]
		truncated = fmt.Sprintf("\n\n… truncated at %d characters. Ask for more with max_chars.", limit)
	}

	header := fmt.Sprintf("%s\nHTTP %d", target, resp.StatusCode)
	if contentType != "" {
		header += " · " + contentType
	}
	if strings.TrimSpace(text) == "" {
		return llm.ToolResult{Output: header + "\n\nThe response body was empty."}
	}
	return llm.ToolResult{Output: header + "\n\n" + strings.TrimSpace(text) + truncated}
}

func handleScreenshotURL(dataDir string) llm.ToolHandler {
	return func(ctx context.Context, workDir string, input json.RawMessage) llm.ToolResult {
		var req struct {
			URL    string `json:"url"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
			WaitMS int    `json:"wait_ms"`
		}
		json.Unmarshal(input, &req)

		target, err := normalizeURL(req.URL)
		if err != nil {
			return llm.ToolResult{Output: err.Error(), IsError: true}
		}
		browser := findBrowser()
		if browser == "" {
			return llm.ToolResult{Output: "No Chrome or Chromium is installed on this machine, so pages " +
				"cannot be photographed. Use fetch_url to read the page, and ask the user to look at it."}
		}

		if req.Width <= 0 {
			req.Width = 1440
		}
		if req.Height <= 0 {
			req.Height = 900
		}
		if req.WaitMS <= 0 {
			req.WaitMS = 4000
		}

		// Written under the agent's own working directory so it can open the
		// result. A screenshot filed somewhere the agent's sandbox cannot read
		// is a screenshot nobody looks at.
		dir := filepath.Join(shotBaseDir(workDir, dataDir), ".openpaw", "screenshots")
		if err := os.MkdirAll(dir, 0755); err != nil {
			return llm.ToolResult{Output: "Could not create a place to save the screenshot: " + err.Error(), IsError: true}
		}
		path := filepath.Join(dir, fmt.Sprintf("%s-%d.png", shotSlug(target), time.Now().Unix()))

		// A throwaway profile: without it headless Chrome opens the user's own
		// profile directory, which fails outright while their browser is running.
		profile, err := os.MkdirTemp("", "openpaw-shot-")
		if err != nil {
			return llm.ToolResult{Output: "Could not create a browser profile: " + err.Error(), IsError: true}
		}
		defer os.RemoveAll(profile)

		ctx, cancel := context.WithTimeout(ctx, screenshotTimeout)
		defer cancel()

		cmd := exec.CommandContext(ctx, browser,
			"--headless=new",
			"--disable-gpu",
			"--hide-scrollbars",
			"--no-first-run",
			"--user-data-dir="+profile,
			fmt.Sprintf("--window-size=%d,%d", req.Width, req.Height),
			// Virtual time lets the page's own timers and fetches run to
			// completion at once, rather than photographing a half-drawn page.
			fmt.Sprintf("--virtual-time-budget=%d", req.WaitMS),
			"--screenshot="+path,
			target,
		)
		// Wait bounded rather than indefinitely: current headless Chrome writes
		// the screenshot and then stays running. Waiting for it to exit means
		// waiting for the timeout on every successful shot, so the image being
		// on disk is what counts as done.
		cmd.WaitDelay = 2 * time.Second

		var stderr strings.Builder
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			return llm.ToolResult{Output: "Could not start the browser: " + err.Error(), IsError: true}
		}
		exited := make(chan error, 1)
		go func() { exited <- cmd.Wait() }()

		written := waitForImage(ctx, path, exited)
		cancel()
		<-exited

		if !written {
			return llm.ToolResult{Output: fmt.Sprintf(
				"The browser produced no image for %s — the page may not have loaded. "+
					"Check it with fetch_url first.\n%s", target, lastLine(stderr.String())), IsError: true}
		}

		return llm.ToolResult{Output: fmt.Sprintf(
			"Photographed %s at %dx%d and saved it to:\n%s\n\n"+
				"OPEN IT NOW with the Read tool — you can see images, and looking at the page is the "+
				"only way to catch what the markup does not show.", target, req.Width, req.Height, path)}
	}
}

// waitForImage returns once the screenshot is fully on disk, or false if the
// browser gave up first.
//
// Stability is checked over two polls rather than trusting the first sight of
// the file: a PNG caught mid-write is a truncated image, and an agent opening
// one sees half a page and reports on it.
func waitForImage(ctx context.Context, path string, exited <-chan error) bool {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	var lastSize int64
	browserGone := false

	for {
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			if info.Size() == lastSize {
				return true
			}
			lastSize = info.Size()
		} else if browserGone {
			// It exited without writing anything, so nothing is coming.
			return false
		}

		select {
		case <-ctx.Done():
			return false
		case <-exited:
			// One more pass: a browser that exits cleanly has already written
			// the file, and returning here would throw away a good screenshot.
			browserGone = true
			exited = nil
		case <-ticker.C:
		}
	}
}

// shotBaseDir prefers the run's working directory, which is inside whatever the
// agent is allowed to read.
func shotBaseDir(workDir, dataDir string) string {
	if strings.TrimSpace(workDir) != "" {
		return workDir
	}
	if strings.TrimSpace(dataDir) != "" {
		return dataDir
	}
	return os.TempDir()
}

// findBrowser locates a Chrome-family browser, preferring the standard macOS
// application bundles before anything on PATH.
func findBrowser() string {
	candidates := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
		"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c
		}
	}
	for _, name := range []string{"google-chrome", "chromium", "chromium-browser", "chrome"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

// normalizeURL accepts what someone would actually type — "localhost:3000" —
// and rejects anything that is not http(s), since a file:// or a custom scheme
// here would be a way to read arbitrary local files through a tool that
// advertises itself as fetching web pages.
func normalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("url is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%q is not a URL: %v", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("only http and https URLs can be fetched, not %q — read local files with Read", u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("%q has no host", raw)
	}
	return u.String(), nil
}

func shotSlug(target string) string {
	u, err := url.Parse(target)
	if err != nil {
		return "page"
	}
	slug := u.Host + u.Path
	var b strings.Builder
	for _, r := range strings.ToLower(slug) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "page"
	}
	if len(out) > 40 {
		out = strings.Trim(out[:40], "-")
	}
	return out
}

var (
	// Written out rather than as one alternation with a backreference: Go's
	// regexp engine has no backreferences, and MustCompile panics at init.
	dropTags = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>` +
		`|<style[^>]*>.*?</style>` +
		`|<noscript[^>]*>.*?</noscript>` +
		`|<svg[^>]*>.*?</svg>` +
		`|<head[^>]*>.*?</head>` +
		`|<!--.*?-->`)
	anyTag    = regexp.MustCompile(`(?s)<[^>]+>`)
	blankRuns = regexp.MustCompile(`\n{3,}`)
	spaceRuns = regexp.MustCompile(`[ \t]{2,}`)
)

// htmlToText strips markup down to what a reader would see. Deliberately crude:
// the point is to answer "did the right content render", and a full parse would
// buy accuracy nobody is asking for here. raw:true is the way out when the
// markup itself is the question.
func htmlToText(html string) string {
	text := dropTags.ReplaceAllString(html, " ")

	// Block boundaries become line breaks before the tags go, otherwise every
	// paragraph in the document runs into the next one.
	for _, tag := range []string{"</p>", "</div>", "</li>", "</tr>", "</h1>", "</h2>", "</h3>", "</h4>", "<br>", "<br/>", "<br />"} {
		text = strings.ReplaceAll(text, tag, tag+"\n")
	}
	text = anyTag.ReplaceAllString(text, "")

	replacer := strings.NewReplacer(
		"&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">",
		"&quot;", "\"", "&#39;", "'", "&mdash;", "—", "&ndash;", "–",
	)
	text = replacer.Replace(text)

	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if s := strings.TrimSpace(spaceRuns.ReplaceAllString(line, " ")); s != "" {
			lines = append(lines, s)
		}
	}
	return blankRuns.ReplaceAllString(strings.Join(lines, "\n"), "\n\n")
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(lines[i]); t != "" {
			return t
		}
	}
	return ""
}
