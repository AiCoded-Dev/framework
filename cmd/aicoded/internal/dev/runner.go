// Package dev is the local runner behind aicoded dev: it builds an app, serves the runner
// protocol to it through the runner server, and fronts it with a gateway that stands in for the
// company login. Its Runner serves the runner protocol alone, to one app that another program
// starts.
package dev

import (
	"context"
	"crypto/rand"
	"fmt"
	"slices"

	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto/egress"
	"aicoded.dev/framework/runnerproto/server"
)

// csrfKeySize is the size of the form key each app instance gets.
const csrfKeySize = server.MinCSRFKey

// newCSRFKey returns a fresh random form key.
func newCSRFKey() []byte {
	key := make([]byte, csrfKeySize)
	_, _ = rand.Read(key)
	return key
}

// providers returns what the local runner gives the app of m through a server: the file stores
// in files, the mail catcher and the spans of st, the values of dev.yaml, the calls of router,
// which may be nil, and the requests to outside services through out.
func providers(m manifest.Manifest, files *fileStore, st *store, v devconfig.AppValues, router *server.Router, out *egress.Client) server.Providers {
	p := server.Providers{Files: files, Mail: st.mail, Secrets: devValues{m: m, v: v}, Telemetry: st, Outbound: out}
	if router != nil {
		p.Calls = router
	}
	return p
}

// devValues hold the values in dev.yaml of the settings and secrets that the app of m declares,
// and hand out no other.
type devValues struct {
	m manifest.Manifest
	v devconfig.AppValues
}

func (d devValues) Secret(_ context.Context, name string) (string, error) {
	if !slices.Contains(d.m.Secrets, name) {
		return "", fmt.Errorf("secret %s is not declared", quote(name))
	}
	return d.v.Secrets[name], nil
}

func (d devValues) Settings() map[string]string {
	out := make(map[string]string, len(d.m.Settings))
	for _, n := range d.m.Settings {
		out[n] = d.v.Settings[n]
	}
	return out
}
