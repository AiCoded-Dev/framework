package errs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	want := New("E-RPC-008", "shop does not call billing.GetInvoice", "run aicoded generate in shop")
	got, ok := Parse(want.Error())
	require.True(t, ok)
	assert.Equal(t, want, got)

	got, ok = Parse("E-MAIL-005: the message is too large")
	require.True(t, ok)
	assert.Equal(t, New("E-MAIL-005", "the message is too large", "read the docs page below"), got)

	for _, msg := range []string{"", "internal error", "e-rpc-008: lower case", "E-RPC-8: short", "no invoice 7: E-RPC-008: later"} {
		_, ok := Parse(msg)
		assert.False(t, ok, msg)
	}
}
