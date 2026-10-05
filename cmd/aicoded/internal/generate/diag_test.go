package generate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
)

func TestDiagnostics(t *testing.T) {
	require.NoError(t, Diagnostics{}.Err())

	b := errs.At("pages/b/index.html:1", "E-GEN-004", "b", "fix b")
	a := errs.At("pages/a/index.html:3", "E-GEN-005", "a", "fix a")
	err := Diagnostics{b, a}.Err()
	require.Error(t, err)
	assert.Equal(t, a.Error()+"\n"+b.Error(), err.Error())
	assert.Equal(t, "E-GEN-005", errs.Code(err))
}
