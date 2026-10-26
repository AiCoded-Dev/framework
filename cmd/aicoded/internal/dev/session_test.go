package dev

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/migrations"
	"aicoded.dev/framework/cmd/aicoded/internal/testhome"
	"aicoded.dev/framework/migrate"
)

// counted is a platform that counts the questions it answers.
type counted struct{ n atomic.Int32 }

func (c *counted) Migrations(context.Context, string) ([]migrate.Step, error) {
	c.n.Add(1)
	return nil, nil
}

func TestSession(t *testing.T) {
	p := &counted{}
	s := newSession(p)
	now := time.Now()
	s.now = func() time.Time { return now }
	ask := func(app string) error {
		_, err := s.Migrations(t.Context(), app)
		return err
	}

	require.NoError(t, ask("notes"))
	require.ErrorIs(t, ask("notes"), migrations.ErrUseCache, "a rebuild takes the cache")
	require.NoError(t, ask("tags"), "each app is asked once")
	s.refresh("notes")
	require.ErrorIs(t, ask("notes"), migrations.ErrUseCache, "a restart within a minute takes the cache")
	now = now.Add(time.Minute)
	s.refresh("notes")
	require.NoError(t, ask("notes"), "a restart a minute later asks again")
	require.ErrorIs(t, ask("notes"), migrations.ErrUseCache)
	assert.Equal(t, int32(3), p.n.Load())
}

// A builder who never signed in, with a configuration folder that others can open, gets one line
// that says so, not a refusal of the folder.
func TestPlatformWithoutASignIn(t *testing.T) {
	home := testhome.Set(t)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".config", "aicoded"), 0o755))
	t.Setenv("AICODED_PLATFORM", "http://127.0.0.1:1")
	steps, warning := migrations.Published(t.Context(), t.TempDir(), "notes", Platform())
	assert.Empty(t, steps)
	assert.Equal(t, "E-CLI-004: not signed in to the platform at http://127.0.0.1:1; only the draft is applied", warning)
}
