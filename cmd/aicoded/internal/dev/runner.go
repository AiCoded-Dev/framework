// Package dev is the local runner behind aicoded dev: it builds an app, serves the runner
// protocol to it through the runner server, and fronts it with a gateway that stands in for the
// company login. Its Runner serves the runner protocol alone, to one app that another program
// starts.
package dev

import (
	"context"
	"crypto/rand"

	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
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

// providers returns what the local runner gives the app of a server: the file stores in files,
// the mail catcher and the spans of st, the values of dev.yaml and the calls of router, which
// may be nil.
func providers(files *fileStore, st *store, v devconfig.AppValues, router *server.Router) server.Providers {
	p := server.Providers{Files: files, Mail: st.mail, Secrets: devValues{v}, Telemetry: st}
	if router != nil {
		p.Calls = router
	}
	return p
}

// devValues hold the values of an app's settings and secrets in dev.yaml.
type devValues struct{ v devconfig.AppValues }

func (d devValues) Secret(_ context.Context, name string) (string, error) {
	return d.v.Secrets[name], nil
}

func (d devValues) Settings() map[string]string { return d.v.Settings }
