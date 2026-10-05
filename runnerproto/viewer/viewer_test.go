package viewer_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/runnerproto/viewer"
)

var now = time.Unix(1_800_000_000, 0)

func claims() viewer.Claims {
	return viewer.Claims{Subject: "alice", Name: "Alice", Roles: []string{"hotel-ops"}, Audience: "rooms",
		IssuedAt: now.Unix(), Expires: now.Add(viewer.MaxLifetime).Unix()}
}

func key(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return pub, priv
}

func sign(t *testing.T, priv ed25519.PrivateKey, c viewer.Claims) string {
	token, err := viewer.Sign(priv, c)
	require.NoError(t, err)
	return token
}

// raw builds a token around any payload, bypassing Sign's checks.
func raw(priv ed25519.PrivateKey, payload []byte) string {
	p := base64.RawURLEncoding.EncodeToString(payload)
	sig := ed25519.Sign(priv, []byte("aicoded-viewer.v1."+p))
	return "v1." + p + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func TestVerify(t *testing.T) {
	pub, priv := key(t)
	otherPub, _ := key(t)
	got, err := viewer.Verify([]ed25519.PublicKey{otherPub, pub}, sign(t, priv, claims()), "rooms", now)
	require.NoError(t, err)
	assert.Equal(t, claims(), got)
}

func TestVerifyRejects(t *testing.T) {
	pub, priv := key(t)
	_, otherPriv := key(t)
	good := sign(t, priv, claims())
	sig := good[strings.LastIndex(good, ".")+1:]
	forged := "v1." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"mallory","aud":"rooms"}`)) + "." + sig
	long := claims()
	long.Expires = long.IssuedAt + 3600
	longPayload, err := json.Marshal(long)
	require.NoError(t, err)
	billing := claims()
	billing.Audience = "billing"

	overflowClaims := claims()
	overflowClaims.Expires = overflowClaims.IssuedAt + 9_223_372_037
	overflowPayload, err := json.Marshal(overflowClaims)
	require.NoError(t, err)

	negativeClaims := viewer.Claims{Subject: "bob", Audience: "rooms", IssuedAt: -10, Expires: 20}
	negativePayload, err := json.Marshal(negativeClaims)
	require.NoError(t, err)

	cases := map[string]struct {
		token string
		at    time.Time
		want  error
	}{
		"empty":                {"", now, viewer.ErrMalformed},
		"wrong version":        {"v2" + good[2:], now, viewer.ErrMalformed},
		"extra part":           {good + ".x", now, viewer.ErrMalformed},
		"oversized":            {"v1." + strings.Repeat("a", 9000) + ".b", now, viewer.ErrMalformed},
		"foreign key":          {sign(t, otherPriv, claims()), now, viewer.ErrSignature},
		"tampered":             {forged, now, viewer.ErrSignature},
		"expired":              {good, now.Add(viewer.MaxLifetime + 10*time.Second), viewer.ErrExpired},
		"not yet valid":        {good, now.Add(-10 * time.Second), viewer.ErrExpired},
		"too long lived":       {raw(priv, longPayload), now, viewer.ErrExpired},
		"other app":            {sign(t, priv, billing), now, viewer.ErrAudience},
		"overflowing lifetime": {raw(priv, overflowPayload), now, viewer.ErrExpired},
		"negative issued-at":   {raw(priv, negativePayload), now, viewer.ErrExpired},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := viewer.Verify([]ed25519.PublicKey{pub}, c.token, "rooms", c.at)
			require.ErrorIs(t, err, c.want)
		})
	}
}

func TestSignRefusesLongLivedTokens(t *testing.T) {
	_, priv := key(t)
	c := claims()
	c.Expires = c.IssuedAt + 3600
	_, err := viewer.Sign(priv, c)
	require.Error(t, err)

	c = claims()
	c.Expires = c.IssuedAt + 9_223_372_037
	_, err = viewer.Sign(priv, c)
	require.Error(t, err)
}

// appClaims are the claims of a call shop makes to billing as itself.
func appClaims() viewer.Claims {
	return viewer.Claims{Audience: "billing", Caller: "shop", App: true,
		IssuedAt: now.Unix(), Expires: now.Add(viewer.MaxLifetime).Unix()}
}

func TestSignChecksTheShape(t *testing.T) {
	_, priv := key(t)
	for name, change := range map[string]func(*viewer.Claims){
		"app token with a subject":   func(c *viewer.Claims) { c.Subject = "alice" },
		"app token with a name":      func(c *viewer.Claims) { c.Name = "Alice" },
		"app token with groups":      func(c *viewer.Claims) { c.Groups = []string{"lisbon"} },
		"app token with roles":       func(c *viewer.Claims) { c.Roles = []string{"admin"} },
		"app token without a caller": func(c *viewer.Claims) { c.Caller = "" },
		"no audience":                func(c *viewer.Claims) { c.Audience = "" },
		"viewer without a subject":   func(c *viewer.Claims) { c.App = false },
	} {
		t.Run(name, func(t *testing.T) {
			c := appClaims()
			change(&c)
			_, err := viewer.Sign(priv, c)
			require.Error(t, err)
		})
	}
	_, err := viewer.Sign(priv, appClaims())
	require.NoError(t, err)
}

func TestVerifyChecksTheShape(t *testing.T) {
	pub, priv := key(t)
	got, err := viewer.Verify([]ed25519.PublicKey{pub}, sign(t, priv, appClaims()), "billing", now)
	require.NoError(t, err)
	assert.Equal(t, appClaims(), got)

	for name, change := range map[string]func(*viewer.Claims){
		"app token with roles":     func(c *viewer.Claims) { c.Roles = []string{"admin"} },
		"app token with a subject": func(c *viewer.Claims) { c.Subject = "alice" },
	} {
		crafted := appClaims()
		change(&crafted)
		payload, err := json.Marshal(crafted)
		require.NoError(t, err)
		_, err = viewer.Verify([]ed25519.PublicKey{pub}, raw(priv, payload), "billing", now)
		require.ErrorIs(t, err, viewer.ErrMalformed, name)
	}
}

func TestVerifyIssued(t *testing.T) {
	pub, priv := key(t)
	_, otherPriv := key(t)
	issued := func(ago time.Duration) viewer.Claims {
		c := claims()
		c.IssuedAt = now.Add(-ago).Unix()
		c.Expires = c.IssuedAt + int64(viewer.MaxLifetime/time.Second)
		return c
	}
	verify := func(token string) (viewer.Claims, error) {
		return viewer.VerifyIssued([]ed25519.PublicKey{pub}, token, "rooms", now, 10*time.Minute)
	}

	got, err := verify(sign(t, priv, issued(9*time.Minute)))
	require.NoError(t, err, "an expired token is accepted until 10 minutes after it was issued")
	assert.Equal(t, issued(9*time.Minute), got)
	for _, ago := range []time.Duration{10 * time.Minute, -5 * time.Second} {
		_, err := verify(sign(t, priv, issued(ago)))
		require.NoError(t, err, "issued %v ago", ago)
	}

	long := issued(0)
	long.Expires = long.IssuedAt + 3600
	longPayload, err := json.Marshal(long)
	require.NoError(t, err)
	billing := issued(0)
	billing.Audience = "billing"
	for name, c := range map[string]struct {
		token string
		want  error
	}{
		"issued 11 minutes ago": {sign(t, priv, issued(11*time.Minute)), viewer.ErrExpired},
		"issued 601 s ago":      {sign(t, priv, issued(10*time.Minute+time.Second)), viewer.ErrExpired},
		"issued in the future":  {sign(t, priv, issued(-10*time.Second)), viewer.ErrExpired},
		"issued 6 s ahead":      {sign(t, priv, issued(-6*time.Second)), viewer.ErrExpired},
		"for another app":       {sign(t, priv, billing), viewer.ErrAudience},
		"foreign key":           {sign(t, otherPriv, issued(0)), viewer.ErrSignature},
		"too long lived":        {raw(priv, longPayload), viewer.ErrExpired},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := verify(c.token)
			require.ErrorIs(t, err, c.want)
		})
	}
}
