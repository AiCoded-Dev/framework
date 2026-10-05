package explain

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/docs"
	"aicoded.dev/framework/internal/errs"
)

func TestListHasEveryPage(t *testing.T) {
	pages, err := fs.Glob(docs.FS, "errors/*.md")
	require.NoError(t, err)
	entries, err := List()
	require.NoError(t, err)
	assert.Len(t, entries, len(pages)-1, "every page but README.md")
	assert.IsIncreasing(t, codes(entries))
	assert.Contains(t, entries, Entry{Code: "E-DEV-001", Title: "dev.yaml is readable by other users"})
}

func codes(entries []Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Code
	}
	return out
}

func TestPage(t *testing.T) {
	page, err := Page("e-dev-001")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(page, "# E-DEV-001: "), page)
}

func TestPageRefuses(t *testing.T) {
	for _, code := range []string{"E-NOPE-001", "E-DEV-1", "../README", ""} {
		_, err := Page(code)
		assert.Equal(t, "E-CLI-001", errs.Code(err), code)
	}
}

func TestRead(t *testing.T) {
	page, err := Read("e-dev-001")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(page, "# E-DEV-001: "), page)

	guide, err := Read("guides/overview")
	require.NoError(t, err)
	want, err := docs.Read("guides/overview")
	require.NoError(t, err)
	assert.Equal(t, want, guide)

	for _, topic := range []string{"E-NOPE-001", "guides/nope", "README", "../README", ""} {
		_, err := Read(topic)
		assert.Equal(t, "E-CLI-001", errs.Code(err), topic)
	}
}
