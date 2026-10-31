package outbound

import (
	"testing"
	"time"
)

// ShortenTimes sets, for the test, the runner's deadline that the client assumes and the
// client's backstop after it.
func ShortenTimes(t *testing.T, runnerDeadline, clientBackstop time.Duration) {
	d, b := deadline, backstop
	deadline, backstop = runnerDeadline, clientBackstop
	t.Cleanup(func() { deadline, backstop = d, b })
}
