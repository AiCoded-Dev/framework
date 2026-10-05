// Package deps is what the pages of the dev UI share: the workspace they show and change.
package deps

import (
	"context"
	"time"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
)

// Backend is the workspace the dev UI shows and changes.
type Backend interface {
	devapi.Backend
	// Stop stops app until Start; it returns at once.
	Stop(app string) error
	// Start starts app, or restarts it when it runs; it returns at once.
	Start(app string) error
	// SetManual switches app into manual mode, or out of it; it returns at once.
	SetManual(app string, on bool) error
	// Personas returns the personas the gateway offers.
	Personas() []devconfig.Persona
	// Changed returns a channel that is closed at the next change of the workspace.
	Changed() <-chan struct{}
}

// Deps is passed to every page's data provider.
type Deps struct {
	Backend Backend
	// Executable is the absolute path of this aicoded, for the MCP setup commands.
	Executable string
}

// New returns the Deps of the pages.
func New(b Backend, executable string) *Deps {
	return &Deps{Backend: b, Executable: executable}
}

// Settle is how long Follow waits after a change before it updates, so that a burst of
// changes makes one update.
const Settle = 250 * time.Millisecond

// Follow calls update at once, and again after every change of b, until ctx is done; it then
// returns nil. A page's Subscribe returns what it returns.
func Follow(ctx context.Context, b Backend, update func(context.Context)) error {
	for {
		changed := b.Changed()
		update(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-changed:
		}
		settle := time.NewTimer(Settle)
		select {
		case <-ctx.Done():
			settle.Stop()
			return nil
		case <-settle.C:
		}
	}
}

// LocalTime returns t in local time as the pages show it: the time of day, after the date when
// t falls on another day than now.
func LocalTime(t, now time.Time) string {
	t, now = t.Local(), now.Local()
	if t.Format(time.DateOnly) == now.Format(time.DateOnly) {
		return t.Format(time.TimeOnly)
	}
	return t.Format(time.DateTime)
}
