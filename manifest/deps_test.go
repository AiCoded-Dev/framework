package manifest_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The package stays light for its importers: the framework, the YAML library, x/mod and,
// through mailrules, protobuf, but never lint.
func TestDependencies(t *testing.T) {
	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", "-f", "{{.ImportPath}} {{with .Module}}{{.Path}}{{end}}", ".").Output()
	require.NoError(t, err)
	allowed := []string{"aicoded.dev/framework", "go.yaml.in/yaml/v3", "golang.org/x/mod", "google.golang.org/protobuf"}
	for line := range strings.Lines(strings.TrimSpace(string(out))) {
		pkg, mod, _ := strings.Cut(strings.TrimSpace(line), " ")
		assert.NotEqual(t, "aicoded.dev/framework/lint", pkg)
		if mod != "" {
			assert.Contains(t, allowed, mod, pkg)
		}
	}
}
