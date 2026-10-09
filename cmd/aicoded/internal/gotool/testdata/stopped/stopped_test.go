package stopped

import (
	"os"
	"syscall"
	"testing"
)

// TestKillsTheGoCommand kills the go command that runs it.
func TestKillsTheGoCommand(t *testing.T) {
	if err := syscall.Kill(os.Getppid(), syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
}
