//go:build unix

package generate

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteModesIgnoreUmask(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "pages"), 0o700))
	g := newGen(App{Dir: dir})
	g.files["pages/new/dir/a.go"] = []byte("package dir\n")
	g.assets.files["pages/x/a.js"] = []byte("x")
	g.snapshots[".aicoded/rpc.json"] = []byte("{}\n")

	defer syscall.Umask(syscall.Umask(0o077))
	require.NoError(t, g.write())

	for p, mode := range map[string]fs.FileMode{
		"pages/new": 0o755, "pages/new/dir": 0o755, "pages/new/dir/a.go": 0o644,
		"pages/assets_gen": 0o755, "pages/assets_gen/pages/x": 0o755, "pages/assets_gen/pages/x/a.js": 0o644,
		".aicoded": 0o755, ".aicoded/rpc.json": 0o644,
	} {
		st, err := os.Stat(filepath.Join(dir, p))
		require.NoError(t, err)
		assert.Equal(t, mode, st.Mode().Perm(), p)
	}
	st, err := os.Stat(filepath.Join(dir, "pages"))
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o700), st.Mode().Perm(), "an existing folder keeps its mode")
}

func TestManifestKeepsItsMode(t *testing.T) {
	dir := copyApp(t, "app")
	manifest := filepath.Join(dir, "aicoded.yaml")
	require.NoError(t, os.Chmod(manifest, 0o640))
	outside := filepath.Join(t.TempDir(), "outside.yaml")
	require.NoError(t, os.WriteFile(outside, []byte("keep\n"), 0o600))
	require.NoError(t, os.Symlink(outside, manifest+".aicoded-tmp"))

	require.NoError(t, Generate(dir))
	st, err := os.Stat(manifest)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o640), st.Mode().Perm())
	assert.NoFileExists(t, manifest+".aicoded-tmp", "a stale temporary file is removed")
	data, err := os.ReadFile(outside)
	require.NoError(t, err)
	assert.Equal(t, "keep\n", string(data), "never written through a link out of the app")
}

func TestSnapshotNeverReadThroughLinks(t *testing.T) {
	dir := rpcApp(t, billingRPC)
	outside := t.TempDir()
	write(t, outside, "rpc.json", "{")
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, ".aicoded")))

	err := Generate(dir)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "E-RPC-005", "the snapshot out of the app is never read")
	assert.Equal(t, "# billing app\napp: billing\n", read(t, dir, "aicoded.yaml"))
	assert.Equal(t, "{", read(t, outside, "rpc.json"))
}

func TestHeldSnapshotsNeverCrossLinks(t *testing.T) {
	dir := shopApp(t)
	outside := filepath.Join(t.TempDir(), "billing.json")
	require.NoError(t, os.WriteFile(outside, []byte("{"), 0o600))
	held := filepath.Join(dir, ".aicoded", "services", "billing.json")
	require.NoError(t, os.Remove(held))
	require.NoError(t, os.Symlink(outside, held))

	err := Generate(dir)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "E-RPC-005", "a held snapshot out of the app is never read")

	require.NoError(t, WriteFile(dir, ".aicoded/services/billing.json", heldBilling.Marshal()))
	data, err := os.ReadFile(outside)
	require.NoError(t, err)
	assert.Equal(t, "{", string(data), "WriteFile replaces the link and never writes through it")
	require.NoError(t, Generate(dir))
}
