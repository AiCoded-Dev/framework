package notes

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"aicoded.dev/framework/web"
)

type guard struct{}

func (guard) Guard(context.Context, *web.Request) error { return nil }

func TestGuardWithoutGuardTrueStopsTheApp(t *testing.T) {
	assert.NotPanics(t, func() { NewRoute(&DP{}) })

	var got any
	func() {
		defer func() { got = recover() }()
		NewRoute(struct {
			*DP
			guard
		}{&DP{}, guard{}})
	}()
	assert.Contains(t, fmt.Sprint(got), "E-WEB-007", "a Guard from an embedded type is never called")
}
