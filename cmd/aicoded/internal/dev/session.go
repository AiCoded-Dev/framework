package dev

import (
	"context"
	"sync"
	"time"

	"aicoded.dev/framework/cmd/aicoded/internal/migrations"
	"aicoded.dev/framework/migrate"
)

// refetchAfter is how old the last answer about an app's published migrations must be before an
// explicit start or restart of the app asks the platform again.
const refetchAfter = time.Minute

// session asks the platform for an app's published migrations at the app's first start under
// aicoded dev, and again at an explicit start or restart once the last answer is a minute old.
// Between them its answer is migrations.ErrUseCache, so a rebuild waits for no request.
type session struct {
	platform migrations.Platform
	now      func() time.Time

	mu    sync.Mutex
	asked map[string]time.Time
	again map[string]bool
}

func newSession(p migrations.Platform) *session {
	return &session{platform: p, now: time.Now, asked: map[string]time.Time{}, again: map[string]bool{}}
}

func (s *session) Migrations(ctx context.Context, app string) ([]migrate.Step, error) {
	s.mu.Lock()
	_, asked := s.asked[app]
	if asked && !s.again[app] {
		s.mu.Unlock()
		return nil, migrations.ErrUseCache
	}
	s.asked[app] = s.now()
	delete(s.again, app)
	s.mu.Unlock()
	return s.platform.Migrations(ctx, app)
}

// refresh has the next start of app ask the platform again, when it last asked a minute ago or
// more.
func (s *session) refresh(app string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if at, ok := s.asked[app]; ok && s.now().Sub(at) >= refetchAfter {
		s.again[app] = true
	}
}
