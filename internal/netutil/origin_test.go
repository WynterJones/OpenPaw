package netutil

import (
	"net/http"
	"testing"
)

func req(host, origin string, headers map[string]string) *http.Request {
	r := &http.Request{Host: host, Header: http.Header{}}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

// The desktop webview and a plain local browser session must keep working
// exactly as they did before the same-origin rewrite.
func TestCheckWSOrigin_Localhost(t *testing.T) {
	cases := []struct{ host, origin string }{
		{"localhost:41295", "http://localhost:41295"},
		{"127.0.0.1:41295", "http://127.0.0.1:41295"},
		{"[::1]:41295", "http://[::1]:41295"},
	}
	for _, c := range cases {
		if !CheckWSOrigin(req(c.host, c.origin, nil)) {
			t.Errorf("host %q origin %q: rejected, want allowed", c.host, c.origin)
		}
	}
}

// The bug this fixes: reaching the app straight at its tailnet address left the
// UI loaded but with no live updates and no terminals.
func TestCheckWSOrigin_TailnetIPDirect(t *testing.T) {
	if !CheckWSOrigin(req("100.64.0.1:41295", "http://100.64.0.1:41295", nil)) {
		t.Error("direct tailnet address rejected")
	}
}

// `tailscale serve` terminates TLS and proxies to the app port, so the Origin
// carries an implicit 443 while the request arrives on 41295.
func TestCheckWSOrigin_TailscaleServe(t *testing.T) {
	r := req("box.tailnet.ts.net", "https://box.tailnet.ts.net", map[string]string{
		"X-Forwarded-Proto": "https",
	})
	if !CheckWSOrigin(r) {
		t.Error("tailscale serve origin rejected")
	}
}

// A front end that rewrites Host records the original in X-Forwarded-Host.
func TestCheckWSOrigin_ForwardedHost(t *testing.T) {
	r := req("127.0.0.1:41295", "https://openpaw.example.com", map[string]string{
		"X-Forwarded-Host": "openpaw.example.com",
	})
	if !CheckWSOrigin(r) {
		t.Error("forwarded host rejected")
	}

	// A proxy chain appends; the original client-facing host is first.
	r = req("127.0.0.1:41295", "https://openpaw.example.com", map[string]string{
		"X-Forwarded-Host": "openpaw.example.com, inner.local",
	})
	if !CheckWSOrigin(r) {
		t.Error("first entry of forwarded host chain rejected")
	}
}

// The whole point of the check: another site must not be able to open a socket
// to this server, whatever address the server is reachable at.
func TestCheckWSOrigin_RejectsForeignOrigins(t *testing.T) {
	cases := []struct{ host, origin string }{
		{"box.tailnet.ts.net", "https://evil.com"},
		{"100.64.0.1:41295", "http://evil.com"},
		{"localhost:41295", "http://localhost.evil.com:41295"},
		{"localhost:41295", "http://evil.com:41295"},
		// Suffix and prefix games must not pass a substring-style comparison.
		{"box.tailnet.ts.net", "https://notbox.tailnet.ts.net"},
		{"box.tailnet.ts.net", "https://box.tailnet.ts.net.evil.com"},
	}
	for _, c := range cases {
		if CheckWSOrigin(req(c.host, c.origin, nil)) {
			t.Errorf("host %q origin %q: allowed, want rejected", c.host, c.origin)
		}
	}
}

// Ports are compared when both sides name one, so a different service on the
// same box cannot borrow the host match.
func TestCheckWSOrigin_PortMismatchRejected(t *testing.T) {
	if CheckWSOrigin(req("localhost:41295", "http://localhost:3000", nil)) {
		t.Error("mismatched explicit ports allowed")
	}
}

// The Vite dev server is legitimately cross-origin and has to stay named.
func TestCheckWSOrigin_ViteDevServer(t *testing.T) {
	if !CheckWSOrigin(req("localhost:41295", "http://localhost:5173", nil)) {
		t.Error("vite dev server rejected")
	}
}

// Non-browser clients send no Origin. Token auth has already run by this point.
func TestCheckWSOrigin_NoOriginAllowed(t *testing.T) {
	if !CheckWSOrigin(req("localhost:41295", "", nil)) {
		t.Error("missing origin rejected")
	}
}

func TestCheckWSOrigin_MalformedOriginRejected(t *testing.T) {
	for _, origin := range []string{"null", "not a url", "http://", "://x"} {
		if CheckWSOrigin(req("localhost:41295", origin, nil)) {
			t.Errorf("origin %q allowed, want rejected", origin)
		}
	}
}

// Hostname comparison is case-insensitive; DNS is.
func TestCheckWSOrigin_CaseInsensitiveHost(t *testing.T) {
	if !CheckWSOrigin(req("Box.Tailnet.TS.net", "https://box.tailnet.ts.net", nil)) {
		t.Error("case difference rejected")
	}
}
