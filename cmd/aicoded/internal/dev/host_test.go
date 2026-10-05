package dev

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/control"
	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/testhome"
	"aicoded.dev/framework/internal/errs"
)

// hostHello makes a workspace with the hello app under a temp home, with its dev.yaml there,
// and returns the workspace's root and gateway.
func hostHello(t *testing.T) (root, gateway string) {
	home := testhome.Set(t)
	root = filepath.Join(home, "work")
	copyHello(t, filepath.Join(root, "hello"), "hello")
	port := freePort(t)
	testhome.DevYAML(t, home, fmt.Sprintf(`workspaces:
  %s:
    port: %d
    apps:
      hello:
        settings: {greeting: hi}
        secrets: {token: abc}
`, root, port))
	return root, fmt.Sprintf("http://localhost:%d/", port)
}

// host runs Host for dir and returns a function that closes the workspace once, also when the
// test ends.
func host(t *testing.T, dir string) func() {
	w, err := Host(t.Context(), dir, io.Discard, Options{})
	require.NoError(t, err)
	closeOnce := sync.OnceFunc(w.Close)
	t.Cleanup(closeOnce)
	return closeOnce
}

func TestHostServesControl(t *testing.T) {
	requireGo(t)
	root, gateway := hostHello(t)
	closeWorkspace := host(t, root)

	state, err := StatePath(root)
	require.NoError(t, err)
	sock := control.SocketPath(state)
	st, err := control.Dial(sock).Status(t.Context())
	require.NoError(t, err)
	assert.Equal(t, root, st.Root)
	assert.Equal(t, gateway, st.Gateway)
	require.Len(t, st.Apps, 1)
	assert.Equal(t, devapi.Running, st.Apps[0].State)
	for path, mode := range map[string]os.FileMode{state: 0o700, sock: 0o600} {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, mode, info.Mode().Perm(), path)
	}

	closeWorkspace()
	assert.NoFileExists(t, sock, "Close removes the socket")
	_, _, ok := FindRunning(t.Context(), root)
	assert.False(t, ok)
}

func TestHostRefusesSecond(t *testing.T) {
	requireGo(t)
	root, gateway := hostHello(t)
	host(t, root)

	for _, dir := range []string{root, filepath.Join(root, "hello")} {
		_, err := Host(t.Context(), dir, io.Discard, Options{})
		assert.Equal(t, "E-DEV-012", errs.Code(err), dir)
		require.ErrorContains(t, err, "aicoded dev already runs for "+root+" at "+gateway)
	}
	found, _, ok := FindRunning(t.Context(), filepath.Join(root, "hello"))
	assert.True(t, ok)
	assert.Equal(t, root, found)
}

func TestHostReadsDevYAMLAtEveryStart(t *testing.T) {
	requireGo(t)
	root, gateway := hostHello(t)
	host(t, root)
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	state, err := StatePath(root)
	require.NoError(t, err)
	c := control.Dial(control.SocketPath(state))

	testhome.DevYAML(t, home, "workspaces:\n  "+root+":\n    apps: {hello: {secrets: {token: s3cret\n")
	a, err := c.Restart(t.Context(), "hello")
	require.NoError(t, err)
	require.NotEmpty(t, a.Problems)
	assert.Equal(t, "E-DEV-002", a.Problems[0].Code)
	assert.NotContains(t, fmt.Sprint(a.Problems), "s3cret", "the problem never quotes dev.yaml")

	testhome.DevYAML(t, home, "workspaces:\n  "+root+":\n    apps: {hello: {settings: {greeting: hey}, secrets: {token: abc}}}\n")
	a, err = c.Restart(t.Context(), "hello")
	require.NoError(t, err)
	assert.Equal(t, devapi.Running, a.State)
	var port int
	_, err = fmt.Sscanf(gateway, "http://localhost:%d/", &port)
	require.NoError(t, err)
	assert.Equal(t, "hey all-roles (token 3 bytes)", gatewayGet(t, port, "hello"))
}

// serveOn writes a dev.yaml under home that serves each of roots on a free port, and returns
// their gateway addresses in order.
func serveOn(t *testing.T, home string, roots ...string) []string {
	var yaml strings.Builder
	var gateways []string
	yaml.WriteString("workspaces:\n")
	for _, root := range roots {
		port := freePort(t)
		fmt.Fprintf(&yaml, "  %s:\n    port: %d\n", root, port)
		gateways = append(gateways, fmt.Sprintf("http://localhost:%d/", port))
	}
	testhome.DevYAML(t, home, yaml.String())
	return gateways
}

func TestHostClaimsTheSocketBeforeItStarts(t *testing.T) {
	home := testhome.Set(t)
	root := filepath.Join(home, "work")
	unbuilt(t, root, "shop")
	serveOn(t, home, root)
	state, err := StateDir(root)
	require.NoError(t, err)
	l, err := control.Listen(t.Context(), control.SocketPath(state))
	require.NoError(t, err, "an aicoded dev that claimed the socket and is still starting")
	defer l.Close()

	var out syncBuffer
	_, err = Host(t.Context(), root, &out, Options{})
	assert.Equal(t, "E-DEV-012", errs.Code(err))
	assert.Empty(t, out.String(), "nothing started")
}

func TestHostRemovesTheSocketWhenOpenFails(t *testing.T) {
	home := testhome.Set(t)
	root := filepath.Join(home, "work")
	unbuilt(t, root, "shop")
	var lc net.ListenConfig
	l, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	testhome.DevYAML(t, home, fmt.Sprintf("workspaces:\n  %s:\n    port: %d\n", root, l.Addr().(*net.TCPAddr).Port))

	_, err = Host(t.Context(), root, io.Discard, Options{})
	assert.Equal(t, "E-DEV-011", errs.Code(err))
	state, err := StatePath(root)
	require.NoError(t, err)
	assert.NoFileExists(t, control.SocketPath(state))
}

func TestHostRefusesAFolderAboveARunningOne(t *testing.T) {
	home := testhome.Set(t)
	root := filepath.Join(home, "work")
	inner := filepath.Join(root, "inner")
	unbuilt(t, inner, "inner")
	gateways := serveOn(t, home, root, inner)
	host(t, inner)

	var out syncBuffer
	_, err := Host(t.Context(), root, &out, Options{})
	assert.Equal(t, "E-DEV-012", errs.Code(err))
	require.ErrorContains(t, err, "aicoded dev already runs for "+inner+" at "+gateways[1])
	assert.Empty(t, out.String())
}

func TestRunningLogin(t *testing.T) {
	home := testhome.Set(t)
	root := filepath.Join(home, "work")
	inner := filepath.Join(root, "inner")
	unbuilt(t, inner, "inner")
	gateways := serveOn(t, home, inner)
	host(t, inner)
	state, err := StatePath(inner)
	require.NoError(t, err)
	token, err := readToken(state, false)
	require.NoError(t, err)

	for _, dir := range []string{inner, filepath.Join(inner, "sub"), root} {
		link, ok := RunningLogin(t.Context(), dir)
		assert.True(t, ok, dir)
		assert.Equal(t, gateways[0]+"_aicoded/login?token="+token, link, dir)
	}
	_, ok := RunningLogin(t.Context(), filepath.Join(home, "other"))
	assert.False(t, ok, "no aicoded dev runs there")

	require.NoError(t, os.Remove(filepath.Join(state, tokenFile)))
	_, ok = RunningLogin(t.Context(), inner)
	assert.False(t, ok, "the token is gone")
	assert.NoFileExists(t, filepath.Join(state, tokenFile), "RunningLogin makes no token")
}
