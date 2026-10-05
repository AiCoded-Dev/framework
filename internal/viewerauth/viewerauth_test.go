package viewerauth_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/identity"
	"aicoded.dev/framework/internal/viewerauth"
	"aicoded.dev/framework/runnerproto/viewer"
)

const callPath = "/aicoded.runner.v1.AppService/Serve"

// seen is what the next handler finds in its request.
type seen struct {
	id            identity.Identity
	token, caller string
	header        string
}

func mint(t *testing.T, key ed25519.PrivateKey, c viewer.Claims) string {
	now := time.Now()
	c.IssuedAt, c.Expires = now.Unix(), now.Add(time.Minute).Unix()
	token, err := viewer.Sign(key, c)
	require.NoError(t, err)
	return token
}

func TestMiddleware(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	_, otherPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	var got *seen
	h := viewerauth.Middleware([]ed25519.PublicKey{pub}, "rooms", callPath, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		got = &seen{identity.From(ctx), identity.Token(ctx), identity.Caller(ctx), r.Header.Get(viewer.Header)}
	}))

	alice := identity.Identity{Subject: "alice", Roles: []string{"hotel-ops"}}
	viewerToken := mint(t, priv, viewer.Claims{Subject: "alice", Roles: []string{"hotel-ops"}, Audience: "rooms"})
	callToken := mint(t, priv, viewer.Claims{Subject: "alice", Roles: []string{"hotel-ops"}, Audience: "rooms", Caller: "shop"})
	appToken := mint(t, priv, viewer.Claims{Audience: "rooms", Caller: "shop", App: true})
	for name, c := range map[string]struct {
		path, token string
		want        *seen
	}{
		"no token":                         {"/", "", nil},
		"garbage":                          {"/", "garbage", nil},
		"foreign key":                      {"/", mint(t, otherPriv, viewer.Claims{Subject: "alice", Audience: "rooms"}), nil},
		"another app":                      {"/", mint(t, priv, viewer.Claims{Subject: "alice", Audience: "billing"}), nil},
		"viewer token on the call path":    {callPath, viewerToken, nil},
		"call token on a page":             {"/", callToken, nil},
		"app token on a page":              {"/rooms", appToken, nil},
		"call token below the call path":   {callPath + "/", callToken, nil},
		"app token below the call path":    {callPath + "/", appToken, nil},
		"viewer token below the call path": {callPath + "/", viewerToken, &seen{id: alice, token: viewerToken}},
		"viewer token on a page":           {"/", viewerToken, &seen{id: alice, token: viewerToken}},
		"call token on the call path":      {callPath, callToken, &seen{id: alice, token: callToken, caller: "shop"}},
		"app token on the call path":       {callPath, appToken, &seen{caller: "shop"}},
	} {
		t.Run(name, func(t *testing.T) {
			got = nil
			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, c.path, nil)
			if c.token != "" {
				r.Header.Set(viewer.Header, c.token)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			code := http.StatusOK
			if c.want == nil {
				code = http.StatusUnauthorized
			}
			assert.Equal(t, code, w.Code)
			assert.Equal(t, c.want, got, "the token header never reaches the app")
		})
	}
}

func TestMiddlewareViewerCannotBeChanged(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	var roles []string
	h := viewerauth.Middleware([]ed25519.PublicKey{pub}, "rooms", callPath, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		identity.From(r.Context()).Roles[0] = "admin"
		roles = identity.From(r.Context()).Roles
	}))
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r.Header.Set(viewer.Header, mint(t, priv, viewer.Claims{Subject: "alice", Roles: []string{"hotel-ops"}, Audience: "rooms"}))
	h.ServeHTTP(httptest.NewRecorder(), r)
	assert.Equal(t, []string{"hotel-ops"}, roles)
}
