package reactive

import "sync"

// Broadcast is an in-process pub/sub fan-out to every subscriber, for events that concern
// every connected page: a counter of users online, a maintenance banner.
//
// Each subscription buffers one value. Publish never blocks and the freshest value wins. Use
// V = struct{} for a plain "something changed" signal.
//
// All methods are safe for concurrent use.
type Broadcast[V any] struct {
	mu   sync.Mutex
	subs map[*BroadcastSub[V]]struct{}
}

// NewBroadcast creates an empty Broadcast.
func NewBroadcast[V any]() *Broadcast[V] {
	return &Broadcast[V]{subs: make(map[*BroadcastSub[V]]struct{})}
}

// Subscribe registers a new subscription. Close it when done, usually with defer.
func (b *Broadcast[V]) Subscribe() *BroadcastSub[V] {
	s := &BroadcastSub[V]{ch: make(chan V, 1)}
	b.mu.Lock()
	b.subs[s] = struct{}{}
	b.mu.Unlock()
	s.cleanup = func() {
		b.mu.Lock()
		delete(b.subs, s)
		b.mu.Unlock()
	}
	return s
}

// Publish sends v to every subscription. It never blocks.
func (b *Broadcast[V]) Publish(v V) {
	b.mu.Lock()
	subs := make([]*BroadcastSub[V], 0, len(b.subs))
	for s := range b.subs {
		subs = append(subs, s)
	}
	b.mu.Unlock()
	for _, s := range subs {
		select {
		case <-s.ch:
		default:
		}
		select {
		case s.ch <- v:
		default:
		}
	}
}

// TotalSubs returns the number of subscriptions.
func (b *Broadcast[V]) TotalSubs() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}

// BroadcastSub is a subscription returned by Broadcast.Subscribe.
type BroadcastSub[V any] struct {
	ch        chan V
	cleanup   func()
	closeOnce sync.Once
}

// Updates returns the delivery channel. Read it in a select with ctx.Done(). The channel is
// never closed.
func (s *BroadcastSub[V]) Updates() <-chan V { return s.ch }

// Close removes the subscription from its Broadcast. Further calls do nothing.
func (s *BroadcastSub[V]) Close() {
	s.closeOnce.Do(func() {
		if s.cleanup != nil {
			s.cleanup()
		}
	})
}
