package lint

import (
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/docs"
)

func TestImportsPage(t *testing.T) {
	page, err := fs.ReadFile(docs.FS, "errors/E-LINT-001.md")
	require.NoError(t, err)
	_, list, ok := strings.Cut(string(page), "A Go file of an app may import ")
	require.True(t, ok, "the page lists what a Go file of an app may import")
	list, _, ok = strings.Cut(list, "(E-LINT-002)")
	require.True(t, ok, "the list ends with the modules of E-LINT-002")
	var named []string
	for _, m := range regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(list, -1) {
		named = append(named, m[1])
	}
	for _, p := range slices.Concat(blocks, std) {
		assert.Contains(t, named, p, "the page names every package apps may import")
	}
	for _, n := range named {
		assert.True(t, slices.Contains(blocks, n) || slices.Contains(std, n) || slices.Contains([]string{Framework, "modules:", "aicoded.yaml"}, n),
			"the page names %s as allowed, which the allow-list lacks", n)
	}
}

func TestBlocks(t *testing.T) {
	assert.Contains(t, Blocks(), Framework+"/web/form")
	for _, b := range Blocks() {
		assert.True(t, allowed[b], "app code may import %s", b)
	}
	Blocks()[0] = "changed"
	assert.Equal(t, Framework+"/app", Blocks()[0], "each call returns a new slice")
}
