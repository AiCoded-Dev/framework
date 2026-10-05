package e2e

import (
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/runnerproto"
)

// An app run by hand: the dev UI shows its command and run folder, the app built and started
// with that folder serves through the gateway, and its spans show live on the traces page.
func TestDevUIManual(t *testing.T) {
	d := openDesk(t, "desk")
	app := d.app(t)
	require.Equal(t, devapi.Manual, app.State)
	status, body, err := d.visit(t, "/")
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, status)
	assert.Contains(t, body, "desk runs by hand and is not running now")

	ctx := browser(t)
	var state, command, runDir string
	require.NoError(t, chromedp.Run(ctx,
		chromedp.Navigate(d.login),
		chromedp.Navigate(d.base+"/apps/desk"),
		chromedp.Text("#state", &state),
		chromedp.Text("#manual pre:nth-of-type(1)", &command),
		chromedp.Text("#manual pre:nth-of-type(2)", &runDir),
		chromedp.Navigate(d.base+"/traces?app=desk"),
	))
	assert.Equal(t, "manual", state)
	assert.Equal(t, app.Command, command)
	assert.Equal(t, app.RunnerDir, runDir)

	bin := filepath.Join(t.TempDir(), "desk")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", bin, ".")
	build.Dir = app.Dir
	out, err := build.CombinedOutput()
	require.NoError(t, err, string(out))
	run := exec.CommandContext(t.Context(), bin)
	run.Env = []string{runnerproto.EnvRunnerDir + "=" + runDir}
	require.NoError(t, run.Start())
	t.Cleanup(func() { _ = run.Wait() })

	require.Eventually(t, func() bool {
		status, body, err := d.visit(t, "/")
		return err == nil && status == http.StatusOK && body == "desk"
	}, 30*time.Second, 100*time.Millisecond, "the gateway serves the app run by hand")
	assert.Eventually(t, func() bool {
		return strings.Contains(d.out.String(), "aicoded dev: desk is ready (run by hand)\n")
	}, 10*time.Second, 50*time.Millisecond, "aicoded dev says the app run by hand is ready")
	require.NoError(t, chromedp.Run(ctx, until(rowWith("#traces", "HTTP GET"))))
	noViolations(t, ctx)
}
