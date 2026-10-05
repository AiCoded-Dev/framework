package socket

import (
	"testing"
	"time"
)

// SetStreamTimeout sets the stream timeout for servers made during the test.
func SetStreamTimeout(t *testing.T, d time.Duration) {
	old := streamTimeout
	streamTimeout = d
	t.Cleanup(func() { streamTimeout = old })
}
