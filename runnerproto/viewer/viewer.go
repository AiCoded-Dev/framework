// Package viewer signs and verifies the short-lived viewer tokens a runner attaches to
// every request it forwards to an app. A token is "v1." + base64url(JSON claims) + "." +
// base64url(Ed25519 signature over "aicoded-viewer.v1." + the encoded claims).
package viewer

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Header is the request header that carries the token.
const Header = "Aicoded-Viewer"

// MaxLifetime is the longest a token may be valid.
const MaxLifetime = 60 * time.Second

const (
	clockSkew   = 5 * time.Second
	maxTokenLen = 8 << 10
	prefix      = "v1."
	sigContext  = "aicoded-viewer.v1."
)

var enc = base64.RawURLEncoding.Strict()

var (
	ErrMalformed = errors.New("viewer token: malformed")
	ErrSignature = errors.New("viewer token: bad signature")
	ErrExpired   = errors.New("viewer token: expired, not yet valid or too long-lived")
	ErrAudience  = errors.New("viewer token: issued for another app")
)

// Claims describe the viewer of one request. A token for a call another app makes as itself
// has App set and names only the calling app: no subject, name, groups or roles.
type Claims struct {
	Subject  string   `json:"sub"`
	Name     string   `json:"name,omitempty"`
	Groups   []string `json:"groups,omitempty"`
	Roles    []string `json:"roles,omitempty"`
	Audience string   `json:"aud"`
	Caller   string   `json:"caller,omitempty"` // calling app, for service calls
	App      bool     `json:"app,omitempty"`    // the calling app calls as itself, with no viewer
	IssuedAt int64    `json:"iat"`
	Expires  int64    `json:"exp"`
}

func (c Claims) lifetimeOK() bool {
	return c.IssuedAt > 0 && c.Expires > c.IssuedAt &&
		c.Expires-c.IssuedAt <= int64(MaxLifetime/time.Second)
}

// shapeOK reports whether c has an audience and names either a viewer, or only the calling app.
func (c Claims) shapeOK() bool {
	if c.Audience == "" {
		return false
	}
	if c.App {
		return c.Caller != "" && c.Subject == "" && c.Name == "" && len(c.Groups) == 0 && len(c.Roles) == 0
	}
	return c.Subject != ""
}

// Sign returns a token for c signed with key.
func Sign(key ed25519.PrivateKey, c Claims) (string, error) {
	if !c.shapeOK() {
		return "", errors.New("viewer token: needs an audience and a subject, or for an app token a caller and no viewer")
	}
	if !c.lifetimeOK() {
		return "", errors.New("viewer token: lifetime must be positive and at most 60s")
	}
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	p := enc.EncodeToString(payload)
	return prefix + p + "." + enc.EncodeToString(ed25519.Sign(key, []byte(sigContext+p))), nil
}

// Verify checks token against keys and returns its claims if it is valid at now for audience.
func Verify(keys []ed25519.PublicKey, token, audience string, now time.Time) (Claims, error) {
	c, err := decode(keys, token, audience)
	if err != nil {
		return Claims{}, err
	}
	n, skew := now.Unix(), int64(clockSkew/time.Second)
	if n < c.IssuedAt-skew || n >= c.Expires+skew {
		return Claims{}, ErrExpired
	}
	return c, nil
}

// VerifyIssued checks a token that was issued for audience and that the app hands back to the
// runner. It accepts the token for maxAge after it was issued, whether or not it has expired
// since, but refuses one issued in the future or with a lifetime over MaxLifetime.
func VerifyIssued(keys []ed25519.PublicKey, token, audience string, now time.Time, maxAge time.Duration) (Claims, error) {
	c, err := decode(keys, token, audience)
	if err != nil {
		return Claims{}, err
	}
	n, skew := now.Unix(), int64(clockSkew/time.Second)
	if c.IssuedAt > n+skew || c.IssuedAt < n-int64(maxAge/time.Second) {
		return Claims{}, ErrExpired
	}
	return c, nil
}

// decode checks everything about token except the time it is used at.
func decode(keys []ed25519.PublicKey, token, audience string) (Claims, error) {
	if len(token) > maxTokenLen || !strings.HasPrefix(token, prefix) {
		return Claims{}, ErrMalformed
	}
	p, s, ok := strings.Cut(token[len(prefix):], ".")
	if !ok || strings.Contains(s, ".") {
		return Claims{}, ErrMalformed
	}
	sig, err := enc.DecodeString(s)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return Claims{}, ErrMalformed
	}
	if !verifyAny(keys, []byte(sigContext+p), sig) {
		return Claims{}, ErrSignature
	}
	payload, err := enc.DecodeString(p)
	if err != nil {
		return Claims{}, ErrMalformed
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil || !c.shapeOK() {
		return Claims{}, ErrMalformed
	}
	if c.Audience != audience {
		return Claims{}, ErrAudience
	}
	if !c.lifetimeOK() {
		return Claims{}, ErrExpired
	}
	return c, nil
}

func verifyAny(keys []ed25519.PublicKey, msg, sig []byte) bool {
	for _, k := range keys {
		if len(k) == ed25519.PublicKeySize && ed25519.Verify(k, msg, sig) {
			return true
		}
	}
	return false
}
