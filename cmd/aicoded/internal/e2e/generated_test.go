package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/check"
	"aicoded.dev/framework/cmd/aicoded/internal/generate"
)

// The checked-in test apps hold exactly what aicoded generate writes, so no test runs on a stale
// page, client, server or snapshot.
func TestTestdataIsGenerated(t *testing.T) {
	abs := func(parts ...string) string {
		dir, err := filepath.Abs(filepath.Join(append([]string{"testdata"}, parts...)...))
		require.NoError(t, err)
		return dir
	}
	rpc := map[string]string{"shop": abs("rpc", "shop"), "billing": abs("rpc", "billing")}
	for dir, ws := range map[string]map[string]string{
		abs("site"):    nil,
		abs("blocks"):  nil,
		abs("desk"):    nil,
		rpc["shop"]:    rpc,
		rpc["billing"]: rpc,
	} {
		_, err := generate.Run(dir, generate.Options{Frozen: true, Workspace: ws})
		assert.NoError(t, err, "run aicoded generate in %s and commit the result", dir)
	}
}

// The examples hold exactly what aicoded generate writes, as the security checks will require.
func TestExamplesAreGenerated(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "examples"))
	require.NoError(t, err)
	ws := map[string]string{
		"contacts":         filepath.Join(root, "contacts"),
		"people":           filepath.Join(root, "people"),
		"room-maintenance": filepath.Join(root, "room-maintenance"),
	}
	for _, dir := range ws {
		_, err := generate.Run(dir, generate.Options{Frozen: true, Workspace: ws})
		assert.NoError(t, err, "run aicoded generate in %s and commit the result", dir)
	}
}

// The examples pass aicoded check --frozen, lint included, as every app must, under an aicoded
// installed from this checkout, which their go.mod replaces the framework with.
func TestExamplesPassCheck(t *testing.T) {
	requireGo(t)
	checkout, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	require.NoError(t, err)
	root := filepath.Join(checkout, "examples")
	r, err := check.Run(t.Context(), root, check.Options{Frozen: true, FrameworkDir: checkout})
	require.NoError(t, err)
	var out strings.Builder
	require.NoError(t, r.WriteText(&out))
	assert.True(t, r.OK(), out.String())
}
