package rpcgen

import (
	"flag"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/rpcschema"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func golden(t *testing.T, got []byte, name string) {
	t.Helper()
	p := filepath.Join("testdata", "golden", name)
	if *update {
		require.NoError(t, os.WriteFile(p, got, 0o600))
	}
	want, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
}

// numbered returns s with the fields of every type numbered 1, 2, … in name order.
func numbered(s rpcschema.Schema) rpcschema.Schema {
	for name, typ := range s.Types {
		for i, f := range slices.Sorted(maps.Keys(typ.Fields)) {
			typ.Fields[f] = rpcschema.Field{Number: i + 1, Type: typ.Fields[f].Type}
		}
		s.Types[name] = typ
	}
	return s
}

// TestServer writes rpc/server_gen.go for testdata/server and runs the tests there, which serve
// calls through the framework's rpc.Server as the app does.
func TestServer(t *testing.T) {
	dir := testModule(t, "server", nil)
	p, ok, err := Parse(dir)
	require.NoError(t, err)
	require.True(t, ok)
	code, err := Server(p, numbered(p.Schema))
	require.NoError(t, err)
	golden(t, code, "server_gen.go.golden")
	write(t, dir, "rpc/server_gen.go", string(code))
	goTest(t, dir)
}
