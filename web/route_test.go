package web

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"aicoded.dev/framework/internal/errs"
)

type plainDP struct{}

type guardedDP struct{}

func (guardedDP) Guard(context.Context, *Request) error { return nil }

type otherGuardDP struct{}

func (otherGuardDP) Guard() bool { return true }

type pointerGuardDP struct{}

func (*pointerGuardDP) Guard(context.Context, *Request) error { return nil }

func TestCheckNoGuard(t *testing.T) {
	assert.NotPanics(t, func() { CheckNoGuard(plainDP{}, "/notes") })
	assert.NotPanics(t, func() { CheckNoGuard(&plainDP{}, "/notes") })

	for name, dp := range map[string]any{
		"own":             guardedDP{},
		"embedded":        struct{ guardedDP }{},
		"other signature": &otherGuardDP{},
		"pointer method":  pointerGuardDP{},
	} {
		var got any
		func() {
			defer func() { got = recover() }()
			CheckNoGuard(dp, "/notes")
		}()
		err, _ := got.(error)
		assert.Equal(t, "E-WEB-007", errs.Code(err), name)
	}
}
