package generate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
)

const page = `<!doctype html><html><head><ssr:access role="*"/><ssr:assets/></head><body>`

func TestPlacement(t *testing.T) {
	assert.Equal(t, []string{
		"pages/a/index.html:1 E-GEN-051",
		"pages/b/index.html:1 E-GEN-051",
		"pages/c/index.html:1 E-GEN-051",
		"pages/index.html:1 E-GEN-051",
	}, diags(t, map[string]string{
		"pages/index.html":     `<!doctype html><html><head><ssr:access role="*"/><div ssr:if="true"><ssr:assets/></div></head><body><ssr:content/></body></html>`,
		"pages/a/index.html":   `<ssr:var name="n" type="[]int"/><ul><li ssr:for="i in n"><ssr:content/></li></ul>`,
		"pages/a/x/index.html": `<p>x</p>`,
		"pages/b/index.html":   `<ssr:var name="on" type="bool"/><p ssr:if="on">a</p><div ssr:else><ssr:content/></div>`,
		"pages/b/y/index.html": `<p>y</p>`,
		"pages/c/index.html":   `<ssr:var name="on" type="bool"/><html ssr:if="on"><body><p>c</p></body></html>`,
	}))
}

func TestPagesNeedHTML(t *testing.T) {
	err := Generate(newApp(t, map[string]string{
		"pages/index.html":     `<ssr:access role="*"/><p>root gate without a document</p>`,
		"pages/a/index.html":   `<p>a bare fragment below a gate</p>`,
		"pages/b/index.html":   page + `<p>b writes its own document</p></body></html>`,
		"pages/c/index.html":   page + `<ssr:content/></body></html>`,
		"pages/c/d/index.html": `<p>inside c's document</p>`,
	}).Dir)
	assert.Equal(t, "E-GEN-052", errs.Code(err))
	require.ErrorContains(t, err, "pages/index.html:1: E-GEN-052")
	require.ErrorContains(t, err, "pages/a/index.html:1: E-GEN-052")
	assert.NotContains(t, err.Error(), "pages/b/", "b writes its own document")
	assert.NotContains(t, err.Error(), "pages/c/", "d renders inside c's document, and c only redirects")
}
