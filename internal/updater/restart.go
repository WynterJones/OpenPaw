package updater

// RestartCh is a package-level channel that signals main.go to perform
// a graceful shutdown followed by syscall.Exec to restart with the new binary.
var RestartCh = make(chan struct{}, 1)

// RequestRestart sends a non-blocking signal on RestartCh.
func RequestRestart() {
	select {
	case RestartCh <- struct{}{}:
	default:
	}
}
