//go:build darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd

package platform_test

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/platform"
	"aicoded.dev/framework/cmd/aicoded/internal/platform/platformtest"
	"aicoded.dev/framework/internal/errs"
)

func TestTokenRefreshesOnce(t *testing.T) {
	p := platformtest.New(t)
	c, _, pages := newClient(t, p)
	p.ExpiresIn(20)
	signIn(t, c, pages)
	p.ExpiresIn(900)
	requests := p.TokenRequests()

	tokens, fails := make([]string, 2), make([]error, 2)
	var wg sync.WaitGroup
	for i := range tokens {
		other := platform.New(p.URL, &bytes.Buffer{})
		wg.Go(func() { tokens[i], fails[i] = other.Token(t.Context()) })
	}
	wg.Wait()
	require.NoError(t, fails[0])
	require.NoError(t, fails[1])
	assert.Equal(t, 1, p.TokenRequests()-requests, "one refreshes, and the other uses its token")
	cr, err := platform.Load(p.URL)
	require.NoError(t, err)
	assert.Equal(t, []string{cr.AccessToken, cr.AccessToken}, tokens)
}

func TestLockWait(t *testing.T) {
	p := platformtest.New(t)
	c, _, _ := newClient(t, p)
	platform.SetLockWait(t, 100*time.Millisecond)
	dir, err := os.UserConfigDir()
	require.NoError(t, err)
	name := filepath.Join(dir, "aicoded", "credentials.lock")

	unlock, err := platform.Lock(t.Context())
	require.NoError(t, err)
	_, err = c.Token(t.Context())
	require.ErrorContains(t, err, name)
	require.ErrorContains(t, platform.Save(p.URL, platform.Credentials{Org: "acme"}), name, "login waits for it too")
	_, _, err = c.Logout(t.Context())
	require.ErrorContains(t, err, name, "and logout")

	unlock()
	_, err = c.Token(t.Context())
	assert.Equal(t, "E-CLI-004", errs.Code(err))
}
