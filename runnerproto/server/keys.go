package server

import (
	"context"
	"sync"
	"time"
)

// keyTTL is how long an instance keeps a bound key it read.
const keyTTL = time.Minute

// keyCache keeps the values of an instance's bound keys for keyTTL after each successful read, so
// that requests to a bound host do not read the secrets store each time. Reads that miss together
// share one read, and a failed read is not kept. It lives in memory only.
type keyCache struct {
	mu   sync.Mutex
	now  func() time.Time
	keys map[string]*keyRead
}

// keyRead is one read of a key: under way until done is closed, then its value or error.
type keyRead struct {
	done  chan struct{}
	value string
	err   error
	at    time.Time
}

func newKeyCache() *keyCache { return &keyCache{now: time.Now, keys: map[string]*keyRead{}} }

// get returns the value of the secret name: the one kept, the one of a read under way, or a new
// one from read.
func (c *keyCache) get(ctx context.Context, name string, read func(context.Context, string) (string, error)) (string, error) {
	c.mu.Lock()
	if r := c.keys[name]; r != nil {
		select {
		case <-r.done:
			if c.now().Sub(r.at) < keyTTL {
				c.mu.Unlock()
				return r.value, nil
			}
		default:
			c.mu.Unlock()
			select {
			case <-r.done:
				return r.value, r.err
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
	}
	r := &keyRead{done: make(chan struct{})}
	c.keys[name] = r
	c.mu.Unlock()

	r.value, r.err = read(ctx, name)
	c.mu.Lock()
	r.at = c.now()
	if r.err != nil {
		delete(c.keys, name)
	}
	close(r.done)
	c.mu.Unlock()
	return r.value, r.err
}
