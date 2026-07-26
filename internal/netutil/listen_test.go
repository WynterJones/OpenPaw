package netutil

import (
	"strings"
	"testing"
)

func TestPlanListeners_DefaultsToLoopbackOnly(t *testing.T) {
	plan := PlanListeners(RemoteOff, "", 41295)
	if len(plan.Addrs) != 1 || plan.Addrs[0] != "127.0.0.1:41295" {
		t.Fatalf("addrs = %v, want loopback only", plan.Addrs)
	}
	if plan.TailscaleIP != "" {
		t.Errorf("unexpected tailscale ip %q", plan.TailscaleIP)
	}
}

// The desktop webview talks to 127.0.0.1. If a remote mode ever dropped the
// loopback listener the app would be unable to reach its own backend.
func TestPlanListeners_AlwaysKeepsLoopbackInTailscaleMode(t *testing.T) {
	plan := PlanListeners(RemoteTailscale, "", 41295)
	if len(plan.Addrs) == 0 {
		t.Fatal("no addresses planned")
	}
	if plan.Addrs[0] != "127.0.0.1:41295" {
		t.Errorf("first addr = %q, want loopback first", plan.Addrs[0])
	}
}

// Asking for Tailscale on a machine without it must degrade to localhost and
// say why, never fail to start or silently expose everything.
func TestPlanListeners_TailscaleUnavailableFallsBackWithWarning(t *testing.T) {
	if GetTailscaleIP() != "" {
		t.Skip("this machine is on a tailnet; fallback path not exercised")
	}

	plan := PlanListeners(RemoteTailscale, "", 41295)
	if len(plan.Addrs) != 1 || plan.Addrs[0] != "127.0.0.1:41295" {
		t.Errorf("addrs = %v, want loopback only", plan.Addrs)
	}
	if plan.Warning == "" {
		t.Error("expected a warning explaining the downgrade")
	}
	if !strings.Contains(plan.Warning, "Tailscale") {
		t.Errorf("warning should name Tailscale: %s", plan.Warning)
	}
}

func TestPlanListeners_LANBindsEverything(t *testing.T) {
	plan := PlanListeners(RemoteLAN, "", 41295)
	if len(plan.Addrs) != 1 || plan.Addrs[0] != "0.0.0.0:41295" {
		t.Errorf("addrs = %v, want 0.0.0.0 only", plan.Addrs)
	}
}

// An explicit OPENPAW_BIND is a deliberate instruction and must win, so
// existing deployments and container setups keep behaving as before.
func TestPlanListeners_ExplicitBindOverridesMode(t *testing.T) {
	plan := PlanListeners(RemoteTailscale, "0.0.0.0", 8080)
	if len(plan.Addrs) != 1 || plan.Addrs[0] != "0.0.0.0:8080" {
		t.Errorf("addrs = %v, want the override only", plan.Addrs)
	}
}

// A loopback override is the default, not a real instruction, so it must not
// suppress a configured remote mode.
func TestPlanListeners_LoopbackOverrideDoesNotSuppressRemote(t *testing.T) {
	plan := PlanListeners(RemoteLAN, "127.0.0.1", 41295)
	if plan.Addrs[0] != "0.0.0.0:41295" {
		t.Errorf("addrs = %v, want the LAN mode to still apply", plan.Addrs)
	}
}

func TestParseRemoteAccess(t *testing.T) {
	cases := map[string]RemoteAccess{
		"":          RemoteOff,
		"off":       RemoteOff,
		"tailscale": RemoteTailscale,
		"lan":       RemoteLAN,
		"nonsense":  RemoteOff,
	}
	for in, want := range cases {
		if got := ParseRemoteAccess(in); got != want {
			t.Errorf("ParseRemoteAccess(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestListen_OpensAndClosesCleanly(t *testing.T) {
	// Port 0 lets the OS pick, so the test never collides with a running app.
	plan := ListenPlan{Addrs: []string{"127.0.0.1:0"}}
	ls, err := Listen(plan)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	if len(ls) != 1 {
		t.Fatalf("got %d listeners, want 1", len(ls))
	}
	for _, l := range ls {
		l.Close()
	}
}

// A half-open set would leave the server listening somewhere it never reports.
func TestListen_ClosesEarlierListenersOnFailure(t *testing.T) {
	first, err := Listen(ListenPlan{Addrs: []string{"127.0.0.1:0"}})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	taken := first[0].Addr().String()

	// Second plan reuses an address already bound by `first`, so it must fail.
	_, err = Listen(ListenPlan{Addrs: []string{"127.0.0.1:0", taken}})
	if err == nil {
		t.Fatal("expected a bind conflict")
	}
	first[0].Close()

	// If the first listener of the failed plan leaked, this would still be in
	// use somewhere; rebinding proves it was released.
	again, err := Listen(ListenPlan{Addrs: []string{taken}})
	if err != nil {
		t.Fatalf("address was not released: %v", err)
	}
	again[0].Close()
}
