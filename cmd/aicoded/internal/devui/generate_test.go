package devui

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/generate"
)

var update = flag.Bool("update", false, "write the generated files of the pages")

// TestPagesAreGenerated fails when the generated files of the pages differ from what their
// templates make; with -update, as make devui runs it, it writes them.
func TestPagesAreGenerated(t *testing.T) {
	_, err := generate.Run(".", generate.Options{
		Module:    "aicoded.dev/framework/cmd/aicoded/internal/devui",
		Workspace: map[string]string{},
		Frozen:    !*update,
	})
	require.NoError(t, err, "run make devui and commit the result")
}
