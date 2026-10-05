package template

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
	"aicoded.dev/framework/cmd/aicoded/internal/generate/template/node"
	"aicoded.dev/framework/internal/errs"
)

func TestBlockNodeKey(t *testing.T) {
	sum := sha256.Sum256([]byte("index.html:3:Loop:1"))
	key := blockNodeKey("index.html", 3, "Loop", 1)
	assert.Equal(t, hex.EncodeToString(sum[:8]), key)
	assert.NotEqual(t, key, blockNodeKey("index.html", 3, "Loop", 2), "blocks on one line differ by ordinal")
	assert.NotEqual(t, key, blockNodeKey("index.html", 3, "SsrCondition", 1))
}

func TestAnnotateBindings(t *testing.T) {
	tpl, err := parse(t, `<p>{{ a }}</p><p>{{ a + b }}</p><div ssr:if="b"><p>{{ a }}</p></div>
<ul ssr:for="x in list"><li>{{ x }}</li></ul><p class="{{ b }}">{{ a }}</p><p>{{ other }}</p>`)
	require.NoError(t, err)
	keyer := func(refs []string, src string) string { return strings.Join(refs, "+") + "|" + src }
	cond := "rk." + blockNodeKey("index.html", 1, "SsrCondition", 1)
	elem := "rk." + blockNodeKey("index.html", 2, "HtmlElement", 2)
	assert.Equal(t, map[string][]string{
		"rk.a|a":       {"a"},
		"rk.a+b|a + b": {"a", "b"},
		cond:           {"b", "a"},
		elem:           {"b", "a"},
	}, tpl.AnnotateBindings(map[string]bool{"a": true, "b": true}, keyer, "rk"))

	nodes := tpl.CollectReactiveNodes()
	assert.Len(t, nodes, 4, "the sites inside the blocks and the loop over a plain list are not live")
	assert.IsType(t, &node.Expression{}, nodes["rk.a|a"])
	assert.IsType(t, &node.SsrCondition{}, nodes[cond])
	assert.IsType(t, &node.HtmlElement{}, nodes[elem])

	tpl, err = parse(t, `<p>{{ a }}</p><p>{{$ a }}</p>`)
	require.NoError(t, err)
	assert.Equal(t, map[string][]string{"a|a": {"a"}, "a|$a": {"a"}}, tpl.AnnotateBindings(map[string]bool{"a": true}, keyer, ""))
}

func TestBoundElements(t *testing.T) {
	tpl, err := parse(t, `<input ssr:bind="q" type="text"><select ssr:bind="s"></select><textarea ssr:bind="t"></textarea>`)
	require.NoError(t, err)
	refs := tpl.GetSsrBindRefs()
	require.Len(t, refs, 3)
	assert.Equal(t, SsrBindRef{File: "pages/x/index.html", Line: 1, VarName: "q", Element: refs[0].Element}, refs[0])
	assert.Equal(t, "select", refs[1].Element.TagName)

	tpl.MarkBoundElements("rk")
	b := gobuf.New()
	tpl.WriteGoCode(b)
	b.WriteConsts()
	assert.Contains(t, b.String(), strconv.Quote(`<input type="text" data-ssr-bind="rk.q" data-ssr-write>`+
		`<select data-ssr-bind="rk.s" data-ssr-write></select><textarea data-ssr-bind="rk.t" data-ssr-write></textarea>`))

	for _, src := range []string{`<div ssr:bind="q"></div>`, `<script ssr:bind="q"></script>`, `<p ssr:bind="q">x</p>`} {
		_, err := parse(t, src)
		assert.Equal(t, "E-GEN-018", errs.Code(err), src)
	}
}

// A script cannot set the value of a file input, nor know an input's type when it is a value.
func TestBoundInputTypes(t *testing.T) {
	for src, msg := range map[string]string{
		`<input type="file" ssr:bind="q">`:    `ssr:bind on <input type="file">; a script cannot set the file an input holds`,
		`<input ssr:bind="q" type=" FILE ">`:  `ssr:bind on <input type="file">; a script cannot set the file an input holds`,
		`<input type="{{ t }}" ssr:bind="q">`: `ssr:bind on an <input> whose type is a value; write a fixed type`,
	} {
		_, err := parse(t, src)
		var e *errs.Error
		if assert.ErrorAs(t, err, &e, src) {
			assert.Equal(t, "E-GEN-018", e.Code, src)
			assert.Equal(t, msg, e.Msg, src)
		}
	}
	for _, src := range []string{`<input ssr:bind="q">`, `<input type="checkbox" ssr:bind="q">`, `<input type="file">`} {
		_, err := parse(t, src)
		assert.NoError(t, err, src)
	}
}

func TestLiveMarkersAreReserved(t *testing.T) {
	for _, src := range []string{
		`<p data-ssr-bind="8a5edab2.count">x</p>`,
		`<input data-ssr-write>`,
		`<div DATA-SSR-BIND="k"></div>`,
		`<p data-ssr-other="1"></p>`,
		`<ssr:form name="f" data-ssr-bind="k"></ssr:form>`,
		`<ssr:form name="f"><ssr:input name="t" data-ssr-write/></ssr:form>`,
	} {
		_, err := parse(t, src)
		assert.Equal(t, "E-GEN-042", errs.Code(err), src)
	}
	_, err := parse(t, `<p data-ssr="x" data-ssrx="y" data-x="z"></p>`)
	assert.NoError(t, err)
}
