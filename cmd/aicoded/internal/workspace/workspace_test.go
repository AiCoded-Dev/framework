package workspace_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/workspace"
	"aicoded.dev/framework/internal/errs"
)

// app writes an app named name into root/rel and returns its folder as Discover names it, with
// the symbolic links in its path resolved.
func app(t *testing.T, root, rel, name string) string {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "aicoded.yaml"), []byte("app: "+name+"\n"), 0o600))
	dir, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	return dir
}

func TestDiscover(t *testing.T) {
	root := t.TempDir()
	shop := app(t, root, "shop", "shop")
	billing := app(t, root, "team/billing", "billing")
	app(t, root, "shop/tools/inner", "inner")
	for _, rel := range []string{".hidden/a", "_old/a", "vendor/a", "node_modules/a", "team/testdata/a"} {
		app(t, root, rel, "skipped")
	}
	apps, err := workspace.Discover(root)
	require.NoError(t, err)
	assert.Equal(t, []string{shop, billing}, apps, "sorted; no app inside an app; skipped folders are not read")

	apps, err = workspace.Discover(shop)
	require.NoError(t, err)
	assert.Equal(t, []string{shop}, apps, "the root may be an app itself")
}

func TestFind(t *testing.T) {
	root := t.TempDir()
	billing := app(t, root, "billing", "billing")
	app(t, root, "shop", "shop")
	dir, err := workspace.Find(root, "billing")
	require.NoError(t, err)
	assert.Equal(t, billing, dir)

	_, err = workspace.Find(root, "reports")
	assert.Equal(t, "E-RPC-007", errs.Code(err))
	app(t, root, "copy", "billing")
	_, err = workspace.Find(root, "billing")
	assert.Equal(t, "E-RPC-007", errs.Code(err), "two apps named billing")
}
