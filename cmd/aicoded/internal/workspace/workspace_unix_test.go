//go:build unix

package workspace_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/workspace"
)

func TestDiscoverThroughLinks(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	shop := app(t, root, "ws/shop", "shop")
	app(t, root, "elsewhere/billing", "billing")
	require.NoError(t, os.Symlink(filepath.Join(root, "ws"), filepath.Join(root, "link")))
	require.NoError(t, os.Symlink("..", filepath.Join(root, "ws", "loop")))
	require.NoError(t, os.Symlink(filepath.Join(root, "elsewhere"), filepath.Join(root, "ws", "billing")))

	apps, err := workspace.Discover(filepath.Join(root, "link"))
	require.NoError(t, err)
	assert.Equal(t, []string{shop}, apps, "a linked root is followed; links below it, a loop among them, are not")
}
