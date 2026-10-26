package migrate_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/lint"
)

// The building blocks and the runtime that apps import know nothing of migrations.
func TestBlocksDoNotImportMigrate(t *testing.T) {
	args := []string{"list", "-deps", "-f", "{{.ImportPath}}"}
	for _, b := range lint.Blocks() {
		args = append(args, b+"/...")
	}
	out, err := exec.CommandContext(t.Context(), "go", args...).Output()
	require.NoError(t, err)
	deps := strings.Fields(string(out))
	assert.Contains(t, deps, lint.Framework+"/sqldb")
	assert.NotContains(t, deps, lint.Framework+"/migrate")
}
