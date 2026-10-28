package publish

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/platform"
	"aicoded.dev/framework/cmd/aicoded/internal/problem"
)

func TestWriteTextChecks(t *testing.T) {
	for checks, want := range map[string]string{
		"all":        "  checks: the security checks of layers L1 to L5, L8 and L9, and the simulated attacks (L7), ran\n",
		"all-but-l7": "  checks: the security checks of layers L1 to L5, L8 and L9 ran; simulated attacks (L7) come later\n",
		"partial":    "  partial checks: not every security check ran; the steps above say which\n",
		"later":      "  checks: later\n",
		"":           "",
	} {
		var b strings.Builder
		require.NoError(t, WriteText(&b, platform.Publish{ID: "pub_x", App: "demo", SHA: "1a2b3c4d", Status: "passed", Checks: checks}))
		assert.Equal(t, "publish pub_x of demo at 1a2b3c4\n"+want+"passed\n", b.String(), checks)
	}
}

func TestWriteTextDecision(t *testing.T) {
	reasons := []problem.Problem{
		{Code: "E-GATE-041", Pos: "aicoded.yaml:9", Message: "mail to partner.example, which the app type internal-tool does not allow"},
		{Code: "E-GATE-044", Pos: ".aicoded/migration.sql", Message: "a destructive schema change: step pub_x#1, which production has not run"},
	}
	for name, tc := range map[string]struct {
		release *platform.Release
		want    string
	}{
		"none":       {nil, ""},
		"R1 due":     {&platform.Release{Tier: "R1", AppType: "internal-tool@builtin", State: "due"}, "  Pre-approved by app type internal-tool: going live.\n"},
		"R1 live":    {&platform.Release{Tier: "R1", AppType: "finance@3", State: "released", Live: true}, "  Pre-approved by app type finance: live.\n"},
		"R1 pending": {&platform.Release{Tier: "R1", AppType: "finance@3", State: "released"}, "  Pre-approved by app type finance: being released.\n"},
		"R1 replaced": {&platform.Release{Tier: "R1", AppType: "finance@3", State: "replaced"}, "  Pre-approved by app type finance.\n" +
			"  Released: replaced by a newer release.\n"},
		"R2 waits": {&platform.Release{Tier: "R2", AppType: "internal-tool@builtin", Reasons: reasons, State: "waiting"}, "  Needs approval:\n" +
			"    E-GATE-041: mail to partner.example, which the app type internal-tool does not allow\n" +
			"    E-GATE-044: a destructive schema change: step pub_x#1, which production has not run\n" +
			"  An approver must approve it before it goes live.\n"},
		"R2 live": {&platform.Release{Tier: "R2", AppType: "internal-tool@builtin", Reasons: reasons[:1], State: "released", Live: true}, "  Needs approval:\n" +
			"    E-GATE-041: mail to partner.example, which the app type internal-tool does not allow\n" +
			"  Released: live.\n"},
		"R2 replaced": {&platform.Release{Tier: "R2", AppType: "internal-tool@builtin", Reasons: reasons[:1], State: "replaced"}, "  Needs approval:\n" +
			"    E-GATE-041: mail to partner.example, which the app type internal-tool does not allow\n" +
			"  Released: replaced by a newer release.\n"},
		"refused":      {&platform.Release{Tier: "R1", AppType: "internal-tool@builtin", State: "refused", Reason: "E-REL-017: outside services are not run yet."}, "  Not released: E-REL-017: outside services are not run yet.\n"},
		"refused bare": {&platform.Release{Tier: "R1", State: "refused"}, "  Not released.\n"},
		"no type":      {&platform.Release{Tier: "R1", State: "due"}, "  Pre-approved: going live.\n"},
		"failed":       {&platform.Release{Tier: "R1", AppType: "internal-tool@builtin", State: "failed", Reason: "the app did not start"}, "  Release failed: the app did not start.\n"},
		"failed bare":  {&platform.Release{Tier: "R2", AppType: "internal-tool@builtin", Reasons: reasons, State: "failed"}, "  Release failed.\n"},
		"R3":           {&platform.Release{Tier: "R3", AppType: "internal-tool@builtin", State: "refused", Reason: "E-REL-021: the source sqldb holds hr data"}, "  Not allowed: E-REL-021: the source sqldb holds hr data.\n"},
		"superseded":   {&platform.Release{Tier: "R2", AppType: "internal-tool@builtin", Reasons: reasons, State: "superseded"}, "  Not released: a newer publish of the app came after it.\n"},
	} {
		var b strings.Builder
		require.NoError(t, WriteText(&b, platform.Publish{ID: "pub_x", App: "demo", SHA: "1a2b3c4d", Status: "passed", Record: 7, Release: tc.release}))
		assert.Equal(t, "publish pub_x of demo at 1a2b3c4\n  change record 7\n"+tc.want+"passed\n", b.String(), name)
	}
}
