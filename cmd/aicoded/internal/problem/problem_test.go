package problem

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"aicoded.dev/framework/cmd/aicoded/internal/generate"
	"aicoded.dev/framework/internal/errs"
)

var (
	compile = errs.At("main.go:4", "E-CHK-002", "undefined: x", "fix the Go code at this line")
	held    = errs.New("E-RPC-013", "shop holds a snapshot of billing that billing no longer matches", "run aicoded rpc add billing in shop")
)

func TestFrom(t *testing.T) {
	both := []Problem{
		{App: "shop", Code: "E-CHK-002", Pos: "main.go:4", Message: "undefined: x", Fix: "fix the Go code at this line", Docs: errs.DocsBase + "E-CHK-002"},
		{App: "shop", Code: "E-RPC-013", Message: held.Msg, Fix: held.Fix, Docs: errs.DocsBase + "E-RPC-013"},
	}
	assert.Equal(t, both, From("shop", generate.Diagnostics{compile, held}))
	assert.Equal(t, both, From("shop", errors.Join(compile, fmt.Errorf("check: %w", held))), "a tree of wrapped errors")
	assert.Equal(t, both[1:], From("shop", fmt.Errorf("check shop: %w", held)))
	assert.Equal(t, []Problem{{App: "shop", Message: "disk full"}}, From("shop", errors.New("disk full")))
	assert.Empty(t, From("shop", nil))
	assert.Equal(t, both[:1], FromErrs("shop", []*errs.Error{compile}))
}

func TestString(t *testing.T) {
	assert.Equal(t, "shop: "+compile.Error(), From("shop", compile)[0].String())
	assert.Equal(t, compile.Error(), From("", compile)[0].String())
	assert.Equal(t, "shop: disk full", Problem{App: "shop", Message: "disk full"}.String())
}
