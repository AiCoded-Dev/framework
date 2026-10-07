package platform

import (
	"testing"
	"time"
)

// Lock takes the lock on the credentials file, as Token does, and returns the function that
// releases it.
var Lock = lock

// SetLockWait sets how long the lock on the credentials file is waited for, until the test ends.
func SetLockWait(t testing.TB, d time.Duration) {
	old := lockWait
	lockWait = d
	t.Cleanup(func() { lockWait = old })
}
