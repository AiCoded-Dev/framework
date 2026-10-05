package template

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/template/node"
)

func parseTemplate(t *testing.T, src string) *Template {
	t.Helper()
	tpl, err := parse(t, src)
	require.NoError(t, err)
	require.NotNil(t, tpl)
	return tpl
}

// texts returns the text nodes of the tree in order.
func texts(nodes []node.Node) []string {
	var out []string
	for _, n := range nodes {
		switch v := n.(type) {
		case *node.Text:
			out = append(out, v.Text)
		case *node.HtmlElement:
			out = append(out, texts(v.Children)...)
		case *node.SsrCondition:
			for _, c := range v.Conditions {
				out = append(out, texts([]node.Node{c.Body})...)
			}
		case *node.Loop:
			out = append(out, texts(v.Children)...)
		case *node.Content:
			out = append(out, texts(v.Children)...)
		}
	}
	return out
}

func TestWhitespaceCollapsed(t *testing.T) {
	for src, want := range map[string][]string{
		"<div>\n    <p>\n        Hello\n    </p>\n</div>":             {" ", " Hello ", " "},
		"<p>\n    Hello    World\n</p>":                               {" Hello World "},
		"<div>\n    <p>A</p>\n    <p>B</p>\n</div>":                   {" ", "A", " ", "B", " "},
		"<ul>\n    <li>A</li>\n    <li>B</li>\n    <li>C</li>\n</ul>": {" ", "A", " ", "B", " ", "C", " "},
		"<div>\n\n    <p>text</p>\n\n</div>":                          {" ", "text", " "},
		"<span>\n    Hello {{ name }}!\n</span>":                      {" Hello ", "! "},
		"<li>Login: <strong>username</strong></li>":                   {"Login: ", "username"},
	} {
		assert.Equal(t, want, texts(parseTemplate(t, src).nodes), src)
	}
}

// Whitespace between inline elements shows as one space, as in the browser.
func TestWhitespaceBetweenElements(t *testing.T) {
	for src, want := range map[string][]string{
		"<b>a</b> <i>b</i>":      {"a", " ", "b"},
		"<b>a</b>\n\t  <i>b</i>": {"a", " ", "b"},
		"<b>a</b><i>b</i>":       {"a", "b"},
		"<p>a\n<ssr:var name=\"x\" type=\"int\"/>\n<b>b</b></p>": {"a ", "b"},
		"<td>&nbsp;</td>": {"\u00a0"},
		"\n<ssr:access role=\"*\"/>\n<b>a</b> <i>b</i>\n": {"a", " ", "b"},
	} {
		assert.Equal(t, want, texts(parseTemplate(t, src).nodes), src)
	}
	assert.Contains(t, pageCode(t, "<p><b>a</b> <i>b</i></p>"), `"<p><b>a</b> <i>b</i></p>"`)
}

func TestWhitespacePreserved(t *testing.T) {
	for _, tag := range []string{"pre", "textarea", "noscript", "xmp"} {
		src := "<" + tag + ">\n    line 1\n    line 2\n</" + tag + ">"
		assert.Equal(t, []string{"\n    line 1\n    line 2\n"}, texts(parseTemplate(t, src).nodes), tag)
	}
	assert.Equal(t, []string{" ", "\n        preserved\n    ", " "},
		texts(parseTemplate(t, "<div>\n    <pre>\n        preserved\n    </pre>\n</div>").nodes))
	assert.Equal(t, []string{"\n    a  ", "\n"},
		texts(parseTemplate(t, "<pre>\n    a  {{ x }}\n</pre>").nodes))
}

func TestWhitespaceDoctype(t *testing.T) {
	tpl := parseTemplate(t, "<!DOCTYPE html>\n<html>\n<head></head>\n<body></body>\n</html>")
	require.IsType(t, &node.HtmlRaw{}, tpl.nodes[0])
	assert.Equal(t, "<!DOCTYPE html>", tpl.nodes[0].(*node.HtmlRaw).Data)
}

func TestWhitespaceAttributeValuesUnchanged(t *testing.T) {
	tpl := parseTemplate(t, `<div class="foo   bar" data-value="a  b"></div>`)
	el := tpl.nodes[0].(*node.HtmlElement)
	assert.Equal(t, "foo   bar", el.Attributes[0].Values[0].(*node.Text).Text)
}

func TestWhitespaceBeforeElse(t *testing.T) {
	tpl := parseTemplate(t, "<div ssr:if=\"a\">yes</div>\n<div ssr:else>no</div>")
	require.Len(t, tpl.nodes, 1)
	assert.NotNil(t, tpl.nodes[0].(*node.SsrCondition).ElseBody)
}
