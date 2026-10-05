package deps

import (
	"sync"
	"time"

	"aicoded.dev/framework/web/reactive"
)

// LastSeen records when each viewer last opened a page, and tells the pages that follow them.
type LastSeen struct {
	mu    sync.Mutex
	at    map[string]time.Time
	topic *reactive.Topic[string, time.Time]
}

// NewLastSeen returns an empty LastSeen.
func NewLastSeen() *LastSeen {
	return &LastSeen{at: map[string]time.Time{}, topic: reactive.NewTopic[string, time.Time]()}
}

// Record notes that login opened a page at t, unless a later time is recorded.
func (s *LastSeen) Record(login string, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.After(s.at[login]) {
		s.at[login] = t
		s.topic.Publish(login, t)
	}
}

// At returns when login last opened a page, and false when they have not since the app started.
func (s *LastSeen) At(login string) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.at[login]
	return t, ok
}

// Follow subscribes to login: the subscription gets the time of every page they open. Close it
// when done.
func (s *LastSeen) Follow(login string) *reactive.TopicSub[time.Time] {
	return s.topic.Subscribe(login)
}
