package publish

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/platform"
)

func TestWriteTextChecks(t *testing.T) {
	for checks, want := range map[string]string{
		"all":        "  checks: the security checks of layers L1 to L5 and the simulated attacks (L7) ran\n",
		"all-but-l7": "  checks: the security checks of layers L1 to L5 ran; simulated attacks (L7) come later\n",
		"partial":    "  partial checks: not every security check ran; the steps above say which\n",
		"later":      "  checks: later\n",
		"":           "",
	} {
		var b strings.Builder
		require.NoError(t, WriteText(&b, platform.Publish{ID: "pub_x", App: "demo", SHA: "1a2b3c4d", Status: "passed", Checks: checks}))
		assert.Equal(t, "publish pub_x of demo at 1a2b3c4\n"+want+"passed\n", b.String(), checks)
	}
}
