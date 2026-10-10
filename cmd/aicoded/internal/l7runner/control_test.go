package l7runner

import (
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/dev"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto/viewer"
)

var rooms = manifest.Manifest{App: "rooms", Access: map[string]manifest.Access{
	"/rooms":           {Require: []string{"staff"}},
	"/rooms/{id}/edit": {Require: []string{"staff", "facilities|hr"}, Guard: true},
}}

func testControl(t *testing.T, m manifest.Manifest) (*control, []ed25519.PublicKey) {
	signer, err := dev.NewSigner()
	require.NoError(t, err)
	return newControl(m, signer, nil), []ed25519.PublicKey{signer.Public()}
}

func TestMint(t *testing.T) {
	c, keys := testControl(t, rooms)
	now := time.Now()

	token, err := c.mint(tokenRequest{Subject: "l7-a", Roles: []string{"staff", "hr"}, Kind: "valid"})
	require.NoError(t, err)
	claims, err := viewer.Verify(keys, token, "rooms", now)
	require.NoError(t, err)
	assert.Equal(t, "l7-a", claims.Subject)
	assert.Equal(t, "l7-a", claims.Name)
	assert.Equal(t, []string{"staff", "hr"}, claims.Roles)
	assert.Empty(t, claims.Groups)
	assert.Equal(t, int64(60), claims.Expires-claims.IssuedAt)

	token, err = c.mint(tokenRequest{Subject: "l7-n", Kind: "expired"})
	require.NoError(t, err)
	_, err = viewer.Verify(keys, token, "rooms", now)
	require.ErrorIs(t, err, viewer.ErrExpired)
	claims, err = viewer.VerifyIssued(keys, token, "rooms", now, time.Hour)
	require.NoError(t, err)
	assert.InDelta(t, now.Add(-time.Minute).Unix(), claims.Expires, 2, "expired a minute ago")

	token, err = c.mint(tokenRequest{Subject: "l7-a", Kind: "audience"})
	require.NoError(t, err)
	_, err = viewer.Verify(keys, token, "rooms", now)
	require.ErrorIs(t, err, viewer.ErrAudience)
	_, err = viewer.Verify(keys, token, "l7-other-app", now)
	require.NoError(t, err)
}

func TestMintForAnAppNamedLikeTheOtherOne(t *testing.T) {
	c, keys := testControl(t, manifest.Manifest{App: otherApp})
	token, err := c.mint(tokenRequest{Subject: "l7-a", Kind: "audience"})
	require.NoError(t, err)
	_, err = viewer.Verify(keys, token, otherApp, time.Now())
	require.ErrorIs(t, err, viewer.ErrAudience, "an audience token is never for the app itself")
}

func TestTokenRefusals(t *testing.T) {
	c, _ := testControl(t, rooms)
	h := newControlServer(c).Handler
	for name, body := range map[string]string{
		"an unknown kind":    `{"subject": "l7-a", "roles": [], "kind": "forged"}`,
		"no kind":            `{"subject": "l7-a", "roles": []}`,
		"an empty subject":   `{"subject": "", "roles": [], "kind": "valid"}`,
		"a long subject":     `{"subject": "` + strings.Repeat("a", 65) + `", "roles": [], "kind": "valid"}`,
		"a role not listed":  `{"subject": "l7-a", "roles": ["admin"], "kind": "valid"}`,
		"a role of no shape": `{"subject": "l7-a", "roles": ["facilities|hr"], "kind": "valid"}`,
		"a wildcard role":    `{"subject": "l7-a", "roles": ["*"], "kind": "valid"}`,
		"an unknown field":   `{"subject": "l7-a", "roles": [], "kind": "valid", "groups": ["x"]}`,
		"two objects":        `{"subject": "l7-a", "roles": [], "kind": "valid"} {}`,
		"a stray brace":      `{"subject": "l7-a", "roles": [], "kind": "valid"}}`,
		"not JSON":           `subject=l7-a`,
		"a body over 16 KiB": `{"subject": "l7-a", "roles": [], "kind": "valid", "x": "` + strings.Repeat("x", maxRequest) + `"}`,
		"too many roles":     `{"subject": "l7-a", "roles": [` + strings.Repeat(`"staff",`, maxRoles) + `"staff"], "kind": "valid"}`,
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/token", strings.NewReader(body)))
		assert.Equal(t, http.StatusBadRequest, w.Code, name)
		var out errorResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out), name)
		assert.NotEmpty(t, out.Error, name)
		assert.NotContains(t, out.Error, "admin", "%s: the error quotes no value", name)
	}

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/token",
		strings.NewReader(`{"subject": "l7-b", "roles": ["staff", "facilities"], "kind": "valid"}`)))
	assert.Equal(t, http.StatusOK, w.Code, "every role the access rules name")
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/token", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}
