package template

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
	"aicoded.dev/framework/internal/errs"
)

func textCode(t *testing.T, text string, insideExpr bool) string {
	t.Helper()
	nodes, err := parseText(text, "index.html", "pages/x/index.html", 1, insideExpr)
	require.NoError(t, err, text)
	b := gobuf.New()
	for _, n := range nodes {
		n.WriteGoCode(b)
	}
	b.WriteConsts()
	return b.String()
}

func TestTextParse(t *testing.T) {
	for in, want := range map[string]string{
		"<h1>Info</h1>\n    {{ info }}":                              "if err := render.Text(w, info); err != nil {\nreturn err\n}",
		"Hello {{ name }}!!!":                                        "if _, err := io.WriteString(w, _html0); err != nil {",
		"{{ (a == 10) && (b > 'abc') || (d - e * 5) != (!b) && c }}": `render.Text(w, (a==10)&&(b>"abc")||(d-e*5)!=(!b)&&c)`,
		"{{ a.Field }}":                                              `render.Text(w, a.Field)`,
		"{{ a.func(123, '345', b) }}":                                `render.Text(w, a.func(123,"345",b))`,
		"{{ a[10] }}":                                                `render.Text(w, a[10])`,
		"{{ f(1,5, 2.5) }}":                                          `render.Text(w, f(1,5,2.5))`,
		"{{ a > 4 ? 'yes' : 'no' }}":                                 `render.Text(w, render.If(a>4, "yes", "no"))`,
		"{{$ a }} {{$ b }}":                                          "if err := render.HTML(w, b); err != nil {",
		"{{a}} and {{b}}":                                            "_html0 = \" and \"",
		"{{ a-1 }}":                                                  "render.Text(w, a-1)",
		"{{ a - 1 }}":                                                "render.Text(w, a-1)",
		"{{ -1 }}":                                                   "render.Text(w, (-1))",
		"{{ a - -1 }}":                                               "render.Text(w, a-(-1))",
		"{{ a < -1.5 }}":                                             "render.Text(w, a<(-1.5))",
		"{{ x[i-1]-f(-2)-(b)-3 }}":                                   "render.Text(w, x[i-1]-f((-2))-(b)-3)",
		"{{ 'a'-1 }}":                                                `render.Text(w, "a"-1)`,
	} {
		assert.Contains(t, textCode(t, in, false), want, in)
	}
	assert.Contains(t, textCode(t, "key, value in array", true), "for key,value:=range array{")
	assert.Contains(t, textCode(t, "value in array", true), "for _,value:=range array{")
}

func TestStringLiterals(t *testing.T) {
	for in, want := range map[string]string{
		`{{ 'it\'s "x"' }}`:         `render.Text(w, "it's \"x\"")`,
		`{{ "a\tb" }}`:              `render.Text(w, "a\tb")`,
		"{{ `a\\\"b` }}":            `render.Text(w, "a\\\"b")`,
		`{{ 'x\" + evil() + \"' }}`: `render.Text(w, "x\" + evil() + \"")`,
		`{{ "x\"); evil(); //" }}`:  `render.Text(w, "x\"); evil(); //")`,
		`{{ 'a  b' }}`:              `render.Text(w, "a  b")`,
	} {
		assert.Contains(t, textCode(t, in, false), want, in)
	}
	_, err := parseText(`{{ 'a\q' }}`, "index.html", "pages/x/index.html", 1, false)
	assert.Equal(t, "E-GEN-015", errs.Code(err))
}

func TestTextParseErrors(t *testing.T) {
	for in, msg := range map[string]string{
		"{{ a":         "cannot parse the expression: unexpected end of text",
		"{{ a @@ b }}": "cannot parse the expression: unexpected character",
		"{{ a + }}":    "cannot parse the expression: unexpected }}",
		"{{ a b }}":    "cannot parse the expression: unexpected name",
		"{{ 1,5 }}":    "cannot parse the expression: unexpected ','",
	} {
		_, err := parseText(in, "index.html", "pages/x/index.html", 1, false)
		var e *errs.Error
		require.ErrorAs(t, err, &e, in)
		assert.Equal(t, "E-GEN-015", e.Code, in)
		assert.Equal(t, msg, e.Msg, in)
	}
}

func TestTextParseErrorPosition(t *testing.T) {
	_, err := parseText("a\nb\n{{ a @@ b }}", "index.html", "pages/x/index.html", 5, false)
	var e *errs.Error
	require.ErrorAs(t, err, &e)
	assert.Equal(t, "pages/x/index.html:7", e.Pos)
	assert.Equal(t, Fix("E-GEN-015"), e.Fix)
}
