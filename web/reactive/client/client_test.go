package client

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Only block values, which the server rendered with the page's escapers, may become markup.
func TestOnlyBlocksBecomeMarkup(t *testing.T) {
	var all strings.Builder
	require.NoError(t, fs.WalkDir(Files, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(Files, p)
		all.Write(data)
		return err
	}))
	src := all.String()
	assert.Equal(t, 1, strings.Count(src, "innerHTML"))
	assert.Contains(t, src, `if (b.kind === "html") {`+"\n    el.innerHTML = b.value;")
	assert.Contains(t, src, "el.textContent = b.value;")
	for _, banned := range []string{"eval(", "new Function", "insertAdjacentHTML", "outerHTML", "document.write",
		"setAttribute(", "srcdoc", "DOMParser", "createContextualFragment", ".href =", "import("} {
		assert.NotContains(t, src, banned)
	}
	_, err := fs.Stat(Files, Entry)
	assert.NoError(t, err)
}

// A call never waits for a connection that will not come back.
func TestCallsFailOnFinalClose(t *testing.T) {
	src, err := fs.ReadFile(Files, "connection.ts")
	require.NoError(t, err)
	for _, want := range []string{
		"this.closed = e.code === 1000 || e.code === 1001;",
		"for (const id of this.closed ? [...this.pending.keys()] : [...this.sent]) this.settle(id, (p) => p.reject(lost()));",
		"if (this.closed) return Promise.reject(lost());",
		`return { status: 0, message: "The connection was lost." };`,
	} {
		assert.Contains(t, string(src), want)
	}
}
