package toolmgr

import (
	"net"
	"testing"
)

func TestAllocatePortSkipsOccupiedPort(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer occupied.Close()

	port := occupied.Addr().(*net.TCPAddr).Port
	if port == 65535 {
		t.Skip("ephemeral listener used the last available TCP port")
	}
	manager := &Manager{
		tools:    make(map[string]*RunningTool),
		nextPort: port,
	}

	got, err := manager.allocatePort()
	if err != nil {
		t.Fatalf("allocatePort: %v", err)
	}
	if got == port {
		t.Fatalf("allocatePort returned occupied port %d", port)
	}
	if got <= port {
		t.Fatalf("allocatePort = %d, want a port above %d", got, port)
	}
}
