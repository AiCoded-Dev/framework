package generate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
)

func TestAssetsNeedSsrAssets(t *testing.T) {
	const top = `<ssr:access role="*"/><ssr:assets/>`
	for name, c := range map[string]struct {
		files map[string]string
		want  string // position of the E-GEN-050 error; "" when the app generates
	}{
		"a layout writes the tags of the page below": {map[string]string{
			"pages/index.html": doc(top + `<ssr:content/>`), "pages/a/index.html": `<p>a</p>`, "pages/a/index.css": `p{color:red}`,
		}, ""},
		"no <ssr:assets/> at all": {map[string]string{
			"pages/index.html": doc(`<ssr:access role="*"/><ssr:content/>`), "pages/a/index.html": `<p>a</p>`, "pages/a/index.css": `p{color:red}`,
		}, "pages/a/index.html:1"},
		"a gate's <ssr:assets/> does not reach the page below": {map[string]string{
			"pages/index.html": doc(top), "pages/a/index.html": doc(`<p>a</p>`), "pages/a/index.ts": `console.log(1)`,
		}, "pages/a/index.html:1"},
		"a page below a gate writes its own": {map[string]string{
			"pages/index.html": doc(top), "pages/a/index.html": doc(`<ssr:assets/><p>a</p>`), "pages/a/index.ts": `console.log(1)`,
		}, ""},
		"<ssr:assets/> below a layout does not write the layout's tags": {map[string]string{
			"pages/index.html": doc(`<ssr:access role="*"/><ssr:content/>`), "pages/index.css": `p{color:red}`,
			"pages/a/index.html": `<ssr:assets/><p>a</p>`,
		}, "pages/index.html:1"},
		"a live page needs the runtime": {map[string]string{
			"pages/index.html": doc(`<ssr:access role="*"/><ssr:var name="n" type="int" reactive="true"/><p>{{ n }}</p>`),
		}, "pages/index.html:1"},
	} {
		err := Generate(newApp(t, c.files).Dir)
		if c.want == "" {
			require.NoError(t, err, name)
			continue
		}
		assert.Equal(t, "E-GEN-050", errs.Code(err), name)
		assert.ErrorContains(t, err, c.want+": E-GEN-050", name)
	}
}

func TestChain(t *testing.T) {
	rs, err := discover(newApp(t, map[string]string{
		"pages/index.html":          `<ssr:access role="*"/><ssr:content/>`,
		"pages/a/index.html":        `<p>gate</p>`,
		"pages/a/b/index.html":      `<ssr:content/>`,
		"pages/a/b/n_id/index.html": `<ssr:access role="*" shared="true"/><p>leaf</p>`,
		"pages/c/d/index.html":      `<p>d</p>`,
	}), noImages)
	require.NoError(t, err)
	byPath := map[string]*Route{}
	for _, r := range rs {
		byPath[r.Path] = r
	}
	assert.Equal(t, []string{"/", "/a/b", "/a/b/n_id"}, paths(chain("/a/b/n_id", byPath)))
	assert.Equal(t, []string{"/", "/a"}, paths(chain("/a", byPath)))
	assert.Equal(t, []string{"/"}, paths(chain("/", byPath)))
	assert.Equal(t, []string{"/", "/c/d"}, paths(chain("/c/d", byPath)), "a folder with no page is skipped")
}
