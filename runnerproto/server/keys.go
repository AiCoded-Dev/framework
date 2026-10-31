package server

import (
	"context"
	"sync"
	"time"
)

const (
	// keyTTL is how long an instance keeps a bound key it read.
	keyTTL = time.Minute
	// keyReadTimeout bounds one read of a bound key.
	keyReadTimeout = 10 * time.Second
)

// keyCache keeps the values of an instance's bound keys for keyTTL after each successful read, so
// that requests to a bound host do not read the secrets store each time. Reads that miss together
// share one read, which no single request's cancellation ends, and a failed read is not kept. It
// lives in memory only.
type keyCache struct {
	mu      sync.Mutex
	now     func() time.Time
	timeout time.Duration
	keys    map[string]*keyRead
}

// keyRead is one read of a key: under way until done is closed, then its value or error.
type keyRead struct {
	done  chan struct{}
	value string
	err   error
	at    time.Time
}

func newKeyCache() *keyCache {
	return &keyCache{now: time.Now, timeout: keyReadTimeout, keys: map[string]*keyRead{}}
}

// get returns the value of the secret name: the one kept, or that of a read under way, which it
// starts with read when there is none. It stops waiting when ctx ends.
func (c *keyCache) get(ctx context.Context, name string, read func(context.Context, string) (string, error)) (string, error) {
	c.mu.Lock()
	r := c.keys[name]
	if r != nil {
		select {
		case <-r.done:
			if c.now().Sub(r.at) >= keyTTL {
				r = nil
			}
		default:
		}
	}
	if r == nil {
		r = &keyRead{done: make(chan struct{})}
		c.keys[name] = r
		go c.read(context.WithoutCancel(ctx), name, r, read)
	}
	c.mu.Unlock()
	select {
	case <-r.done:
		return r.value, r.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// read runs r, giving read c.timeout, and forgets r when it fails.
func (c *keyCache) read(ctx context.Context, name string, r *keyRead, read func(context.Context, string) (string, error)) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	type result struct {
		value string
		err   error
	}
	got := make(chan result, 1)
	go func() {
		v, err := read(ctx, name)
		got <- result{v, err}
	}()
	var res result
	select {
	case res = <-got:
	case <-ctx.Done():
		res.err = ctx.Err()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	r.value, r.err, r.at = res.value, res.err, c.now()
	if r.err != nil && c.keys[name] == r {
		delete(c.keys, name)
	}
	close(r.done)
}
