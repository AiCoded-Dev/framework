package reactive

import "sync"

// Topic is a keyed in-process pub/sub fan-out. Use it to wake up Subscribe goroutines when
// something they care about changes elsewhere in the process. K is the routing key (for
// example a user id) and V is the payload.
//
// Each subscription buffers one value. Publish never blocks and the freshest value wins: a
// value still buffered is replaced by the new one, so a slow subscriber sees only the latest.
//
// Use V = struct{} as a dirty bit ("something changed for K, query again"), or a concrete V
// when the publisher already has the new value.
//
// All methods are safe for concurrent use.
type Topic[K comparable, V any] struct {
	mu   sync.Mutex
	subs map[K]map[*TopicSub[V]]struct{}
}

// NewTopic creates an empty Topic.
func NewTopic[K comparable, V any]() *Topic[K, V] {
	return &Topic[K, V]{subs: make(map[K]map[*TopicSub[V]]struct{})}
}

// Subscribe registers a new subscription for key. Close it when done, usually with defer.
func (t *Topic[K, V]) Subscribe(key K) *TopicSub[V] {
	s := &TopicSub[V]{ch: make(chan V, 1)}
	t.mu.Lock()
	set, ok := t.subs[key]
	if !ok {
		set = make(map[*TopicSub[V]]struct{}, 1)
		t.subs[key] = set
	}
	set[s] = struct{}{}
	t.mu.Unlock()
	s.cleanup = func() {
		t.mu.Lock()
		if set, ok := t.subs[key]; ok {
			delete(set, s)
			if len(set) == 0 {
				delete(t.subs, key)
			}
		}
		t.mu.Unlock()
	}
	return s
}

// Publish sends v to every subscription for key. It never blocks.
func (t *Topic[K, V]) Publish(key K, v V) {
	t.mu.Lock()
	set := t.subs[key]
	subs := make([]*TopicSub[V], 0, len(set))
	for s := range set {
		subs = append(subs, s)
	}
	t.mu.Unlock()
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

// Len returns the number of keys with at least one subscription.
func (t *Topic[K, V]) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.subs)
}

// TotalSubs returns the number of subscriptions across all keys.
func (t *Topic[K, V]) TotalSubs() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := 0
	for _, set := range t.subs {
		n += len(set)
	}
	return n
}

// TopicSub is a subscription returned by Topic.Subscribe.
type TopicSub[V any] struct {
	ch        chan V
	cleanup   func()
	closeOnce sync.Once
}

// Updates returns the delivery channel. Read it in a select with ctx.Done(). The channel is
// never closed.
func (s *TopicSub[V]) Updates() <-chan V { return s.ch }

// Close removes the subscription from its Topic. Further calls do nothing.
func (s *TopicSub[V]) Close() {
	s.closeOnce.Do(func() {
		if s.cleanup != nil {
			s.cleanup()
		}
	})
}
