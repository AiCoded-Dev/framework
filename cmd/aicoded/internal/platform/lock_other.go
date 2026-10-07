//go:build !(darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd)

package platform

import (
	"context"
	"time"
)

// lockFile takes no lock on a system without flock.
func lockFile(context.Context, string, time.Duration) (func(), error) {
	return func() {}, nil
}
