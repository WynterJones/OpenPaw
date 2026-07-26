package netutil

import (
	"fmt"
	"net"
)

// RemoteAccess is how far the server opens itself up.
type RemoteAccess string

const (
	// RemoteOff is loopback only — nothing off this machine can connect.
	RemoteOff RemoteAccess = "off"
	// RemoteTailscale adds a listener on the tailnet address only. The local
	// network still cannot reach it, which is the main reason to prefer this
	// over binding everything.
	RemoteTailscale RemoteAccess = "tailscale"
	// RemoteLAN binds every interface. Anything that can route to this machine
	// can reach the login page, including other devices on a café's wifi.
	RemoteLAN RemoteAccess = "lan"
)

func ParseRemoteAccess(s string) RemoteAccess {
	switch RemoteAccess(s) {
	case RemoteTailscale:
		return RemoteTailscale
	case RemoteLAN:
		return RemoteLAN
	}
	return RemoteOff
}

// ListenPlan is the set of addresses to accept connections on, plus why.
type ListenPlan struct {
	Addrs []string
	// TailscaleIP is non-empty when a tailnet listener is included, so callers
	// can show the URL a phone should use.
	TailscaleIP string
	// Warning explains a downgrade — e.g. Tailscale was requested but no
	// tailnet address exists on this machine yet.
	Warning string
}

// PlanListeners works out where to listen.
//
// Loopback is always included, even in remote modes. The desktop webview talks
// to 127.0.0.1, so binding solely to the tailnet address would leave the app
// unable to reach its own backend. Two explicit listeners give exactly
// "loopback plus tailnet" without the blanket exposure of 0.0.0.0.
func PlanListeners(mode RemoteAccess, bindOverride string, port int) ListenPlan {
	// An explicit bind (env var or a user typing one in) wins outright — it is
	// a deliberate instruction and should not be second-guessed.
	if bindOverride != "" && bindOverride != "127.0.0.1" && bindOverride != "localhost" {
		return ListenPlan{Addrs: []string{fmt.Sprintf("%s:%d", bindOverride, port)}}
	}

	loopback := fmt.Sprintf("127.0.0.1:%d", port)

	switch mode {
	case RemoteLAN:
		return ListenPlan{Addrs: []string{fmt.Sprintf("0.0.0.0:%d", port)}}

	case RemoteTailscale:
		ip := GetTailscaleIP()
		if ip == "" {
			return ListenPlan{
				Addrs:   []string{loopback},
				Warning: "Remote access is set to Tailscale, but no tailnet address was found — is Tailscale running and logged in? Staying on localhost.",
			}
		}
		return ListenPlan{
			Addrs:       []string{loopback, fmt.Sprintf("%s:%d", ip, port)},
			TailscaleIP: ip,
		}
	}

	return ListenPlan{Addrs: []string{loopback}}
}

// Listen opens every address in the plan. If any fails, the ones already open
// are closed so the caller never ends up half-listening.
func Listen(plan ListenPlan) ([]net.Listener, error) {
	var out []net.Listener
	for _, addr := range plan.Addrs {
		l, err := net.Listen("tcp", addr)
		if err != nil {
			for _, open := range out {
				open.Close()
			}
			return nil, fmt.Errorf("listen on %s: %w", addr, err)
		}
		out = append(out, l)
	}
	return out, nil
}
