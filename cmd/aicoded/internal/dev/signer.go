package dev

import (
	"crypto/ed25519"
	"crypto/rand"
	"time"

	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/runnerproto/viewer"
)

// Signer mints viewer tokens for personas; it stands in for the company login.
type Signer struct {
	pub  ed25519.PublicKey
	priv ed25519.PrivateKey
}

// NewSigner returns a signer with a fresh key.
func NewSigner() (*Signer, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Signer{pub: pub, priv: priv}, nil
}

// Public returns the key apps verify tokens with.
func (s *Signer) Public() ed25519.PublicKey { return s.pub }

// Mint returns a viewer token that lets persona p use app.
func (s *Signer) Mint(p devconfig.Persona, app string, now time.Time) (string, error) {
	return viewer.Sign(s.priv, viewer.Claims{Subject: p.Name, Name: p.Name, Groups: p.Groups, Roles: p.Roles,
		Audience: app, IssuedAt: now.Unix(), Expires: now.Add(viewer.MaxLifetime).Unix()})
}

// MintCall returns the token of a call from the app caller to the app callee: on behalf of the
// viewer v, or of the calling app itself when v is nil.
func (s *Signer) MintCall(v *viewer.Claims, caller, callee string, now time.Time) (string, error) {
	c := viewer.Claims{Audience: callee, Caller: caller, IssuedAt: now.Unix(), Expires: now.Add(viewer.MaxLifetime).Unix()}
	if v == nil {
		c.App = true
	} else {
		c.Subject, c.Name, c.Groups, c.Roles = v.Subject, v.Name, v.Groups, v.Roles
	}
	return viewer.Sign(s.priv, c)
}
