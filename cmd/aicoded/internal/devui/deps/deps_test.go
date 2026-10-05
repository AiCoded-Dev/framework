package deps_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devui/deps"
	"aicoded.dev/framework/cmd/aicoded/internal/devui/fake"
)

func TestFollow(t *testing.T) {
	b := &fake.Backend{}
	ctx, cancel := context.WithCancel(t.Context())
	updates := make(chan time.Time, 10)
	done := make(chan error, 1)
	go func() {
		done <- deps.Follow(ctx, b, func(context.Context) { updates <- time.Now() })
	}()
	<-updates

	changed := time.Now()
	b.Change()
	b.Change()
	b.Change()
	assert.GreaterOrEqual(t, (<-updates).Sub(changed), deps.Settle, "an update waits for the changes to settle")
	select {
	case <-updates:
		assert.Fail(t, "a burst of changes makes one update")
	case <-time.After(2 * deps.Settle):
	}

	cancel()
	require.NoError(t, <-done)
}

func TestLocalTime(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local)
	assert.Equal(t, "08:05:09", deps.LocalTime(time.Date(2026, 9, 30, 8, 5, 9, 0, time.Local), now))
	assert.Equal(t, "2026-09-29 23:59:59", deps.LocalTime(time.Date(2026, 9, 29, 23, 59, 59, 0, time.Local), now))
}
