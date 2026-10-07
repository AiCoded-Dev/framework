//go:build darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd

package platform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// lockFile takes an exclusive flock on the file name, which it makes with mode 0600 when it is
// missing, and returns the function that releases it. It tries again until it gets it, ctx ends
// or wait has passed.
func lockFile(ctx context.Context, name string, wait time.Duration) (func(), error) {
	f, err := os.OpenFile(filepath.Clean(name), os.O_RDONLY|os.O_CREATE|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", name, err)
	}
	fail := func(err error) (func(), error) {
		_ = f.Close()
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		return fail(err)
	}
	if !fi.Mode().IsRegular() {
		return fail(fmt.Errorf("%s is not a file: delete it", name))
	}
	deadline := time.Now().Add(wait)
	for pause := 10 * time.Millisecond; ; pause = min(2*pause, 250*time.Millisecond) {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		switch {
		case err == nil:
			return func() { _ = f.Close() }, nil
		case !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR):
			return fail(fmt.Errorf("lock %s: %w", name, err))
		case !time.Now().Before(deadline):
			return fail(fmt.Errorf("another aicoded has held %s for %s: try again once it has finished", name, wait))
		}
		t := time.NewTimer(min(pause, time.Until(deadline)))
		select {
		case <-ctx.Done():
			t.Stop()
			return fail(ctx.Err())
		case <-t.C:
		}
	}
}
