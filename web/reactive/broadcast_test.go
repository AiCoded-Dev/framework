package reactive

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBroadcastLifecycle(t *testing.T) {
	b := NewBroadcast[int]()
	assert.Equal(t, 0, b.TotalSubs())

	sub := b.Subscribe()
	assert.Equal(t, 1, b.TotalSubs())

	b.Publish(42)
	assert.Equal(t, 42, next(t, sub.Updates()))

	sub.Close()
	assert.Equal(t, 0, b.TotalSubs())
}

func TestBroadcastPublishWithoutSubscribers(t *testing.T) {
	b := NewBroadcast[struct{}]()
	b.Publish(struct{}{})
	assert.Equal(t, 0, b.TotalSubs())
}

func TestBroadcastFanOut(t *testing.T) {
	b := NewBroadcast[string]()
	subs := make([]*BroadcastSub[string], 5)
	for i := range subs {
		subs[i] = b.Subscribe()
		defer subs[i].Close()
	}

	b.Publish("hello")
	for _, s := range subs {
		assert.Equal(t, "hello", next(t, s.Updates()))
	}
}

func TestBroadcastBurstKeepsFreshest(t *testing.T) {
	b := NewBroadcast[int]()
	sub := b.Subscribe()
	defer sub.Close()
	goroutines := runtime.NumGoroutine()

	const n = 10_000
	start := time.Now()
	for i := range n {
		b.Publish(i)
	}
	assert.Less(t, time.Since(start), 5*time.Second)
	assert.Equal(t, n-1, next(t, sub.Updates()))
	assert.LessOrEqual(t, runtime.NumGoroutine(), goroutines+5)
}

func TestBroadcastConcurrentChurn(t *testing.T) {
	b := NewBroadcast[int]()
	var wg sync.WaitGroup
	for w := range 32 {
		wg.Go(func() {
			for i := range 200 {
				sub := b.Subscribe()
				b.Publish(w*1000 + i)
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
	assert.Equal(t, 0, b.TotalSubs())
}

func TestBroadcastCloseTwice(t *testing.T) {
	b := NewBroadcast[int]()
	sub := b.Subscribe()
	sub.Close()
	sub.Close()
	assert.Equal(t, 0, b.TotalSubs())
}

func TestBroadcastConcurrentClose(t *testing.T) {
	b := NewBroadcast[int]()
	sub := b.Subscribe()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(sub.Close)
	}
	wg.Wait()
	assert.Equal(t, 0, b.TotalSubs())
}
