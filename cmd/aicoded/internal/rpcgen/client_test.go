package rpcgen

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/rpcschema"
	"aicoded.dev/framework/internal/errs"
)

func TestPackageName(t *testing.T) {
	name, err := PackageName("my-app")
	require.NoError(t, err)
	assert.Equal(t, "myapp", name)
	for _, app := range []string{"go", "type", "string", "ma-in", "Billing"} {
		_, err := PackageName(app)
		assert.Equal(t, "E-RPC-006", errs.Code(err), app)
	}

	names, err := PackageNames([]string{"billing", "my-app"})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"billing": "billing", "my-app": "myapp"}, names)
	_, err = PackageNames([]string{"a-b", "ab"})
	assert.Equal(t, "E-RPC-006", errs.Code(err), "two clients named ab")
	assert.Contains(t, err.Error(), ".aicoded/services/ab.json:1: E-RPC-006", "at the held snapshot")
}

func TestAddedPackageName(t *testing.T) {
	name, err := AddedPackageName("ab", []string{"ab", "billing"})
	require.NoError(t, err)
	assert.Equal(t, "ab", name, "an app held already may be added again")

	for app, held := range map[string][]string{"ab": {"a-b"}, "go": nil} {
		_, err := AddedPackageName(app, held)
		var e *errs.Error
		require.ErrorAs(t, err, &e, app)
		assert.Equal(t, "E-RPC-006", e.Code, app)
		assert.Empty(t, e.Pos, "the snapshot of %s is not held yet", app)
	}
	_, err = AddedPackageName("reports", []string{"a-b", "ab"})
	assert.Contains(t, err.Error(), ".aicoded/services/ab.json:1: E-RPC-006", "a clash among the held apps is at their snapshot")
}

// TestClient writes the server of testdata/server and the client of its snapshot into one
// module and runs the tests in testdata/client, which call through a runner that hands every
// call to the server.
func TestClient(t *testing.T) {
	src, err := os.ReadFile("testdata/server/rpc/billing.go")
	require.NoError(t, err)
	dir := testModule(t, "client", map[string][]byte{"rpc/billing.go": src})
	p, ok, err := Parse(dir)
	require.NoError(t, err)
	require.True(t, ok)
	s := numbered(p.Schema)
	s.App = "billing"
	server, err := Server(p, s)
	require.NoError(t, err)
	client, err := Client("billing", s)
	require.NoError(t, err)
	golden(t, client, "client_gen.go.golden")
	write(t, dir, "rpc/server_gen.go", string(server))
	write(t, dir, "services/billing/client_gen.go", string(client))
	goTest(t, dir)
}

func TestClientRefusesUnexportedTypes(t *testing.T) {
	_, err := Client("billing", rpcschema.Schema{App: "billing", Types: map[string]rpcschema.Type{"wire": {}}})
	assert.Equal(t, "E-RPC-005", errs.Code(err), "a type named like an import of the client")
}
