package reactive

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// next returns the value buffered in ch. Publish delivers before it returns, so the value
// must already be there.
func next[V any](t *testing.T, ch <-chan V) V {
	t.Helper()
	require.Len(t, ch, 1)
	return <-ch
}

func TestTopicLifecycle(t *testing.T) {
	tp := NewTopic[uint32, int]()
	assert.Equal(t, 0, tp.Len())

	sub := tp.Subscribe(42)
	assert.Equal(t, 1, tp.Len())
	assert.Equal(t, 1, tp.TotalSubs())

	tp.Publish(42, 7)
	assert.Equal(t, 7, next(t, sub.Updates()))

	sub.Close()
	assert.Equal(t, 0, tp.Len())
	assert.Equal(t, 0, tp.TotalSubs())
}

func TestTopicPublishWithoutSubscribers(t *testing.T) {
	tp := NewTopic[string, struct{}]()
	tp.Publish("nobody-home", struct{}{})
	assert.Equal(t, 0, tp.Len())
}

func TestTopicKeyIsolation(t *testing.T) {
	tp := NewTopic[uint32, string]()
	a, b := tp.Subscribe(1), tp.Subscribe(2)
	defer a.Close()
	defer b.Close()

	tp.Publish(1, "for-a")
	assert.Equal(t, "for-a", next(t, a.Updates()))
	assert.Empty(t, b.Updates())
}

func TestTopicSubscriptionsOnOneKey(t *testing.T) {
	tp := NewTopic[uint32, int]()
	s1, s2 := tp.Subscribe(7), tp.Subscribe(7)
	defer s1.Close()
	defer s2.Close()

	tp.Publish(7, 99)
	assert.Equal(t, 99, next(t, s1.Updates()))
	assert.Equal(t, 99, next(t, s2.Updates()))
}

func TestTopicBurstKeepsFreshest(t *testing.T) {
	tp := NewTopic[uint32, int]()
	sub := tp.Subscribe(1)
	defer sub.Close()
	goroutines := runtime.NumGoroutine()

	const n = 10_000
	start := time.Now()
	for i := range n {
		tp.Publish(1, i)
	}
	assert.Less(t, time.Since(start), 5*time.Second)
	assert.Equal(t, n-1, next(t, sub.Updates()))
	assert.LessOrEqual(t, runtime.NumGoroutine(), goroutines+5)
}

func TestTopicConcurrentChurn(t *testing.T) {
	tp := NewTopic[uint32, int]()
	var wg sync.WaitGroup
	for w := range 32 {
		wg.Go(func() {
			for i := range 200 {
				key := uint32((w*13 + i*7) % 16)
				sub := tp.Subscribe(key)
				tp.Publish(key, i)
				if i%3 == 0 {
					select {
					case <-sub.Updates():
					default:
					}
				}
				sub.Close()
			}
		})
	}
	wg.Wait()
	assert.Equal(t, 0, tp.Len())
	assert.Equal(t, 0, tp.TotalSubs())
}

func TestTopicCloseTwice(t *testing.T) {
	tp := NewTopic[uint32, int]()
	sub := tp.Subscribe(1)
	sub.Close()
	sub.Close()
	assert.Equal(t, 0, tp.TotalSubs())
}

func TestTopicConcurrentClose(t *testing.T) {
	tp := NewTopic[uint32, int]()
	sub := tp.Subscribe(1)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(sub.Close)
	}
	wg.Wait()
	assert.Equal(t, 0, tp.TotalSubs())
}

func TestTopicDirtyBitCoalesces(t *testing.T) {
	tp := NewTopic[uint32, struct{}]()
	sub := tp.Subscribe(1)
	defer sub.Close()

	tp.Publish(1, struct{}{})
	tp.Publish(1, struct{}{})
	next(t, sub.Updates())
	assert.Empty(t, sub.Updates())
}
