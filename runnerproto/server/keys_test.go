package server

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeyReadOutlivesTheRequestThatStartedIt(t *testing.T) {
	c := newKeyCache()
	var reads atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	read := func(ctx context.Context, _ string) (string, error) {
		if reads.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			return "key", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	first, cancel := context.WithCancel(t.Context())
	firstErr := make(chan error)
	go func() {
		_, err := c.get(first, "k", read)
		firstErr <- err
	}()
	<-started
	type result struct {
		v   string
		err error
	}
	waiter := make(chan result)
	go func() {
		v, err := c.get(t.Context(), "k", read)
		waiter <- result{v, err}
	}()

	cancel()
	require.ErrorIs(t, <-firstErr, context.Canceled, "the cancelled request stops waiting")
	close(release)
	assert.Equal(t, result{v: "key"}, <-waiter)
	assert.Equal(t, int32(1), reads.Load(), "the waiter shares the read")
}

func TestKeyReadTimesOut(t *testing.T) {
	assert.Equal(t, 10*time.Second, newKeyCache().timeout)
	c := newKeyCache()
	c.timeout = 10 * time.Millisecond
	hang := make(chan struct{})
	t.Cleanup(func() { close(hang) })
	errc := make(chan error, 1)
	go func() {
		_, err := c.get(t.Context(), "k", func(context.Context, string) (string, error) {
			<-hang
			return "key", nil
		})
		errc <- err
	}()
	select {
	case err := <-errc:
		require.ErrorIs(t, err, context.DeadlineExceeded, "a provider that hangs")
	case <-time.After(5 * time.Second):
		require.Fail(t, "the read did not time out")
	}

	v, err := c.get(t.Context(), "k", func(context.Context, string) (string, error) { return "key", nil })
	require.NoError(t, err)
	assert.Equal(t, "key", v, "a read that timed out is not kept")
}
