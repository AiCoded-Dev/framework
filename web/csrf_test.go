package web

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"aicoded.dev/framework/internal/identity"
	"aicoded.dev/framework/internal/runner"
)

var testKey = []byte("0123456789abcdef0123456789abcdef")

func TestCSRF(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	tok := mintCSRF(testKey, "notes", "alice", now)
	assert.True(t, verifyCSRF(testKey, "notes", "alice", tok, now))
	assert.True(t, verifyCSRF(testKey, "notes", "alice", tok, now.Add(csrfLifetime)))
	assert.True(t, verifyCSRF(testKey, "notes", "alice", tok, now.Add(-csrfSkew+time.Second)))
	assert.NotEqual(t, tok, mintCSRF(testKey, "notes", "alice", now), "nonce")

	assert.False(t, verifyCSRF(testKey, "notes", "bob", tok, now), "other viewer")
	assert.False(t, verifyCSRF(testKey, "other", "alice", tok, now), "other app")
	assert.False(t, verifyCSRF([]byte("another key, also 32 bytes long!"), "notes", "alice", tok, now), "other key")
	assert.False(t, verifyCSRF(testKey, "notes", "alice", tok, now.Add(csrfLifetime+time.Second)), "expired")
	assert.False(t, verifyCSRF(testKey, "notes", "alice", tok, now.Add(-csrfSkew-time.Second)), "from the future")
	assert.False(t, verifyCSRF(testKey, "notes", "", mintCSRF(testKey, "notes", "", now), now), "no viewer")

	parts := strings.Split(tok, ".")
	later := parts[0] + "." + "1800003600" + "." + parts[2]
	assert.False(t, verifyCSRF(testKey, "notes", "alice", later, now), "changed issue time")
	for _, bad := range []string{"", "a.b", "a.b.c", tok + ".x", strings.Repeat("A", 200), parts[0] + "." + parts[1] + "."} {
		assert.False(t, verifyCSRF(testKey, "notes", "alice", bad, now), "%q", bad)
	}
}

func TestRequestCSRFToken(t *testing.T) {
	r := &Request{session: &runner.Session{App: "notes", CSRFKey: testKey}, viewer: identity.Identity{Subject: "alice"}}
	tok := r.CSRFToken()
	assert.Equal(t, tok, r.CSRFToken())
	assert.True(t, verifyCSRF(testKey, "notes", "alice", tok, time.Now()))
}
