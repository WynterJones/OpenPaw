package netutil

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// devServerOrigin is the Vite dev server. It is the one genuinely cross-origin
// caller we accept: the page is served from :5173 while the API and WebSockets
// live on the app port, so the same-origin rule below cannot cover it.
const devServerOrigin = "http://localhost:5173"

// CheckWSOrigin reports whether a WebSocket upgrade may proceed.
//
// This used to be an allowlist of literal localhost URLs built from the app
// port. That is correct only while the browser is on the same machine. Reach
// the app over a tailnet — either directly at http://100.x.y.z:41295 or through
// `tailscale serve` at https://box.tailnet.ts.net — and the browser sends that
// address as the Origin, which matched nothing and got the upgrade rejected.
// The REST API kept working, so the app loaded and then sat there with no live
// updates and no terminals at all.
//
// The rule is same-origin instead of a fixed list: the Origin's host must be
// the host the request was addressed to. A page on evil.com opening a socket to
// your tailnet address still sends Origin: https://evil.com against
// Host: box.tailnet.ts.net, so it is still refused — but every address that is
// legitimately *this* server now works without being enumerated in advance.
//
// Ports are compared only when both sides name one. Behind a TLS-terminating
// proxy the Origin carries an implicit 443 while the proxied request arrives at
// the app port, and requiring those to agree would reject the very setup this
// is meant to support.
func CheckWSOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// Non-browser client (CLI, test harness). Browsers always send Origin,
		// so this cannot be used to bypass the check from a page. Token auth
		// has already run by the time we get here regardless.
		return true
	}
	if origin == devServerOrigin {
		return true
	}

	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return sameHost(u.Host, requestHost(r))
}

// requestHost is the address the client believes it connected to. A proxy that
// rewrites Host records the original in X-Forwarded-Host; `tailscale serve`
// preserves Host, but other front ends do not, and trusting the header is safe
// here because the WebSocket API gives page JavaScript no way to set request
// headers at all.
func requestHost(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		// A proxy chain appends, so the original is first.
		if i := strings.IndexByte(fwd, ','); i >= 0 {
			fwd = fwd[:i]
		}
		if fwd = strings.TrimSpace(fwd); fwd != "" {
			return fwd
		}
	}
	return r.Host
}

// sameHost compares two host[:port] pairs. Hostnames must match outright; ports
// must match only when both are present, per the proxy reasoning above.
func sameHost(a, b string) bool {
	ah, ap := splitHostPort(a)
	bh, bp := splitHostPort(b)
	if ah == "" || ah != bh {
		return false
	}
	if ap != "" && bp != "" {
		return ap == bp
	}
	return true
}

// splitHostPort tolerates a missing port, which net.SplitHostPort treats as an
// error, and normalises case and IPv6 brackets so the two sides are comparable.
func splitHostPort(hostport string) (host, port string) {
	if h, p, err := net.SplitHostPort(hostport); err == nil {
		return strings.ToLower(h), p
	}
	return strings.ToLower(strings.Trim(hostport, "[]")), ""
}
