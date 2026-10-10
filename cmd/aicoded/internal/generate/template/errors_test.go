package template

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
	"aicoded.dev/framework/internal/errs"
)

func parse(t *testing.T, src string) (*Template, error) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "index.html")
	require.NoError(t, os.WriteFile(p, []byte(src), 0o600))
	return Parse(p, "pages/x/index.html", func(s string) (string, error) { return s, nil })
}

func pageCode(t *testing.T, src string) string {
	t.Helper()
	tpl, err := parse(t, src)
	require.NoError(t, err, src)
	b := gobuf.New()
	tpl.WriteGoCode(b)
	b.WriteConsts()
	return b.String()
}

func TestErrors(t *testing.T) {
	for src, want := range map[string]string{
		"<p class=\"x></p>":                             "E-GEN-001",
		"<p></p></div>":                                 "E-GEN-001",
		"<ssr:var type=\"int\"/>":                       "E-GEN-002",
		"<ssr:var name=\"a\"/>":                         "E-GEN-003",
		"<ssr:bogus/>":                                  "E-GEN-004",
		"<p ssr:bogus=\"x\"></p>":                       "E-GEN-005",
		"<ssr:form name=\"a\" ssr:if=\"x\"></ssr:form>": "E-GEN-005",
		"<p ssr:else></p>":                              "E-GEN-006",
		"<p ssr:if=\"a\"></p><p ssr:else></p><p ssr:else-if=\"b\"></p>": "E-GEN-006",
		"<p ssr:for=\"x\"></p>":                                            "E-GEN-007",
		"<p ssr:for=\"x in y\" ssr:if=\"a\"></p>":                          "E-GEN-007",
		"<p ssr:if=\"a\" ssr:for=\"x in y\"></p>":                          "E-GEN-007",
		"<p ssr:if=\"a\"></p><p ssr:else-if=\"b\" ssr:for=\"x in y\"></p>": "E-GEN-007",
		"<p ssr:if=\"a\"></p><p ssr:for=\"x in y\" ssr:else></p>":          "E-GEN-007",
		"<p ssr:for=\"x in y\" ssr:for=\"z in y\"></p>":                    "E-GEN-001",
		"<p ssr:if=\"a\" ssr:else></p>":                                    "E-GEN-006",
		"<p ssr:if=\"a\"></p><p ssr:else-if=\"b\" ssr:else></p>":           "E-GEN-006",
		"<ssr:input name=\"a\"/>":                                          "E-GEN-008",
		"<ssr:form name=\"a\"><ssr:form name=\"b\"></ssr:form></ssr:form>": "E-GEN-009",
		"<ssr:form name=\"a\" enctype=\"text/plain\"></ssr:form>":          "E-GEN-010",
		"<ssr:form></ssr:form>":                                            "E-GEN-010",
		"<ssr:form name=\"a b\"></ssr:form>":                               "E-GEN-010",
		"<ssr:form name=\"a\"></ssr:form><ssr:form name=\"A\"></ssr:form>": "E-GEN-010",
		"<ssr:form name=\"a\" enctype=\"application/x-www-form-urlencoded\"><ssr:input name=\"f\" type=\"file\"/></ssr:form>":                                                          "E-GEN-011",
		"<ssr:form name=\"a\"><ssr:input name=\"n\" gotype=\"complex64\"/></ssr:form>":                                                                                                 "E-GEN-012",
		"<ssr:form name=\"a\"><ssr:input name=\"n\" type=\"radio\" gotype=\"int\" value=\"x\"/></ssr:form>":                                                                            "E-GEN-012",
		"<ssr:form name=\"a\"><ssr:input name=\"n\" type=\"radio\" gotype=\"float64\" value=\"NaN\"/></ssr:form>":                                                                      "E-GEN-012",
		"<ssr:form name=\"a\"><ssr:input name=\"n\" type=\"checkbox\" gotype=\"int\" value=\"1\"/><ssr:input name=\"n\" type=\"checkbox\" gotype=\"bool\" value=\"true\"/></ssr:form>": "E-GEN-013",
		"<ssr:form name=\"a\"><ssr:input name=\"n\"/><ssr:input name=\"n\"/></ssr:form>":                                                                                               "E-GEN-014",
		"<ssr:form name=\"a\"><ssr:input name=\"n\"/><ssr:input name=\"n\" type=\"checkbox\"/></ssr:form>":                                                                             "E-GEN-014",
		"<ssr:form name=\"a\"><ssr:input name=\"1n\"/></ssr:form>":                                                                                                                     "E-GEN-014",
		"<p>{{ a + }}</p>":          "E-GEN-015",
		"<p ssr:if=\"x in y\"></p>": "E-GEN-015",
		"<ssr:form name=\"a\"><ssr:input name=\"n\" ssr:bind=\"x\"/></ssr:form>": "E-GEN-018",
		"<ssr:content/><ssr:content/>":                                           "E-GEN-039",
	} {
		_, err := parse(t, "\n"+src)
		assert.Equal(t, want, errs.Code(err), src)
		var e *errs.Error
		if assert.ErrorAs(t, err, &e, src) {
			assert.Equal(t, "pages/x/index.html:2", e.Pos, src)
			assert.NotEmpty(t, e.Fix, src)
			assert.Equal(t, Fix(want), e.Fix, src)
		}
	}
}

func TestOneWrapperPerElement(t *testing.T) {
	for src, msg := range map[string]string{
		`<p ssr:if="a" ssr:for="x in y"></p>`:       "ssr:for cannot share an element with ssr:if; wrap one of them in another element",
		`<p ssr:for="x in y" ssr:else></p>`:         "ssr:for cannot share an element with ssr:else; wrap one of them in another element",
		`<p ssr:if="a" ssr:else></p>`:               "ssr:if and ssr:else cannot share an element",
		`<p ssr:for="x in y" ssr:for="z in y"></p>`: "<p> has the attribute ssr:for twice",
	} {
		_, err := parse(t, src)
		var e *errs.Error
		if assert.ErrorAs(t, err, &e, src) {
			assert.Equal(t, msg, e.Msg, src)
		}
	}
}

func TestFixes(t *testing.T) {
	retired := map[int]bool{44: true}
	assert.Len(t, fixes, 55-len(retired))
	for n := 1; n <= 55; n++ {
		code := fmt.Sprintf("E-GEN-%03d", n)
		if retired[n] {
			assert.Empty(t, Fix(code), code)
			continue
		}
		assert.NotEmpty(t, Fix(code), code)
	}
}

func TestErrAtWithoutFix(t *testing.T) {
	assert.PanicsWithValue(t, "template: no fix line for E-GEN-999", func() { _ = (&parser{}).fail("E-GEN-999", "x") })
	assert.PanicsWithValue(t, "template: no fix line for E-GEN-999", func() { _ = errAt("index.html", 1, "E-GEN-999", "a fix", "x") })
	assert.PanicsWithValue(t, "template: no fix line for E-GEN-001", func() { _ = errAt("index.html", 1, "E-GEN-001", "", "x") })
}

func TestMissingTemplate(t *testing.T) {
	tpl, err := Parse(filepath.Join(t.TempDir(), "index.html"), "pages/x/index.html", nil)
	require.NoError(t, err)
	assert.Nil(t, tpl)
}

func TestReadError(t *testing.T) {
	_, err := Parse(t.TempDir(), "pages/x/index.html", nil)
	require.ErrorContains(t, err, "could not read template file")
	assert.Empty(t, errs.Code(err), "an I/O error is not a markup error")
}

func TestEscapers(t *testing.T) {
	for src, want := range map[string]string{
		`<p>{{ x }}</p>`:            "render.Text(w, x)",
		`<p class="a {{ x }}"></p>`: "render.Attr(w, x)",
		`<p>{{$ x }}</p>`:           "render.HTML(w, x)",
		`<ssr:form name="a" data-x="{{ x }}"></ssr:form>`:                           "render.Attr(w, x)",
		`<ssr:form name="a"><ssr:input name="n" placeholder="{{ x }}"/></ssr:form>`: "render.Attr(w, x)",
		`<ssr:form name="a"><ssr:input name="n"/></ssr:form>`:                       "render.Attr(w, v)",
		`<ssr:form name="a"><ssr:textarea name="n"/></ssr:form>`:                    "render.Text(w, textarea.GetValue())",
	} {
		assert.Contains(t, pageCode(t, src), want, src)
	}
}

func TestForm(t *testing.T) {
	out := pageCode(t, `<ssr:form name="a">&lt;script&gt; &amp; <ssr:input name="n"/></ssr:form>`)
	assert.Contains(t, out, "form := s.\n")
	assert.Contains(t, out, `<input type=\"hidden\" name=\"_aicoded_form\" value=\"`)
	assert.Contains(t, out, "render.Attr(w, form.ID())")
	assert.Contains(t, out, `\"><input type=\"hidden\" name=\"_aicoded_csrf\" value=\"`)
	assert.Contains(t, out, "render.Attr(w, form.Token())")
	assert.Contains(t, out, "&lt;script&gt; &amp; ", "text in a form is escaped")
	assert.NotContains(t, out, "<script>")
}

func TestRadioValues(t *testing.T) {
	radio := `<ssr:form name="a"><ssr:input name="n" type="radio" gotype="%[1]s" value="%[2]s"/>` +
		`<ssr:input name="n" type="radio" gotype="%[1]s" value="%[3]s"/></ssr:form>`
	for _, c := range []struct{ gotype, a, b, want string }{
		{"string", "a", "b&quot;", `input.GetValue() == "a"`},
		{"string", "a", "b&quot;", `input.GetValue() == "b\""`},
		{"int", "1", "2", "input.GetValue() == 1 {"},
		{"int", "007", "+2", "input.GetValue() == 7 {"},
		{"int", "007", "+2", "input.GetValue() == 2 {"},
		{"bool", "1", "false", "input.GetValue() == true {"},
		{"float32", "0.1", "1e3", "input.GetValue() == 1000 {"},
	} {
		out := pageCode(t, fmt.Sprintf(radio, c.gotype, c.a, c.b))
		assert.Contains(t, out, c.want, c)
	}
	out := pageCode(t, fmt.Sprintf(radio, "string", "&quot;&gt;&lt;x", "b"))
	assert.Contains(t, out, `value=\"&#34;&gt;&lt;x\"`, "the HTML value is escaped")
	out = pageCode(t, `<ssr:form name="a"><ssr:input name="n" type="checkbox" gotype="int" value="1"/>`+
		`<ssr:input name="n" type="checkbox" gotype="int" value="2"/></ssr:form>`)
	assert.Contains(t, out, "if _, exists := input.GetValue()[2]; exists {")
}

func TestNodePositions(t *testing.T) {
	out := pageCode(t, "<p>\n\n{{ x }}</p>")
	assert.Contains(t, out, "//line index.html:3\n")
}

func TestContentAndAssets(t *testing.T) {
	out := pageCode(t, `<ssr:assets/><ssr:content/>`)
	assert.Contains(t, out, "if err := s.WriteAssets(w); err != nil {")
	assert.Contains(t, out, "if err := s.WriteChild(w); err != nil {")
}

func TestEndTags(t *testing.T) {
	out := pageCode(t, `<ul><li>a<li>b</ul><p>c</p>`)
	assert.Contains(t, out, `_html0 = "<ul><li>a<li>b</li></li></ul><p>c</p>"`)
	out = pageCode(t, `<ssr:form name="a"><ssr:textarea name="t"></ssr:textarea></ssr:form><p>x</p>`)
	assert.Contains(t, out, `_html4 = "</textarea>"`)
	assert.Contains(t, out, "_html5 = \"</form>\"\n_html6 = \"<p>x</p>\"")
}

func TestImages(t *testing.T) {
	p := filepath.Join(t.TempDir(), "index.html")
	require.NoError(t, os.WriteFile(p, []byte("\n<img src=\"a.png\"><img src=\"b.png\">"), 0o600))
	tpl, err := Parse(p, "pages/x/index.html", func(src string) (string, error) {
		if src == "b.png" {
			return "", errs.New("E-GEN-035", "b.png is missing", "put the file next to the template")
		}
		return "/_aicoded/assets/" + src, nil
	})
	assert.Nil(t, tpl)
	var e *errs.Error
	require.ErrorAs(t, err, &e)
	assert.Equal(t, "pages/x/index.html:2", e.Pos)
	assert.Equal(t, "E-GEN-035", e.Code)

	require.NoError(t, os.WriteFile(p, []byte(`<img src="a.png">`), 0o600))
	tpl, err = Parse(p, "pages/x/index.html", func(src string) (string, error) { return "/_aicoded/assets/" + src, nil })
	require.NoError(t, err)
	b := gobuf.New()
	tpl.WriteGoCode(b)
	b.WriteConsts()
	assert.Contains(t, b.String(), `<img src=\"/_aicoded/assets/a.png\">`)

	_, err = Parse(p, "pages/x/index.html", func(string) (string, error) { return "", errors.New("disk") })
	require.EqualError(t, err, "pages/x/index.html:1: disk")

	tpl, err = Parse(p, "pages/x/index.html", nil)
	require.NoError(t, err)
	b = gobuf.New()
	tpl.WriteGoCode(b)
	b.WriteConsts()
	assert.Contains(t, b.String(), `<img src=\"a.png\">`, "a nil resolver keeps src")
}

func TestAccessAttribute(t *testing.T) {
	for src, want := range map[string]string{
		`<ssr:access/>`:                                "E-GEN-031",
		`<ssr:access role=""/>`:                        "E-GEN-031",
		`<ssr:access role="*,admin"/>`:                 "E-GEN-031",
		`<ssr:access role="a b"/>`:                     "E-GEN-031",
		`<ssr:access role="a" guard="yes"/>`:           "E-GEN-031",
		`<ssr:access role="a"/><ssr:access role="b"/>`: "E-GEN-031",
		`<ssr:access role="a,"/>`:                      "E-GEN-031",
		`<ssr:access role="{{ x }}"/>`:                 "E-GEN-031",
		`<ssr:access role="a" guard/>`:                 "E-GEN-031",
		`<ssr:access role="a" shared="yes"/>`:          "E-GEN-031",
		`<ssr:access role="a" shared/>`:                "E-GEN-031",
		`<ssr:access role="a" roles="b"/>`:             "E-GEN-031",
		`<ssr:access role="a" ssr:if="x"/>`:            "E-GEN-031",
	} {
		_, err := parse(t, src)
		assert.Equal(t, want, errs.Code(err), src)
	}
}

func TestAccess(t *testing.T) {
	tpl, err := parse(t, `<p>open</p>`)
	require.NoError(t, err)
	assert.Nil(t, tpl.Access())

	tpl, err = parse(t, "<p>a</p>\n<ssr:access role=\" editor ,admin\" guard=\"true\"/>")
	require.NoError(t, err)
	assert.Equal(t, &Access{Roles: []string{"editor", "admin"}, Guard: true, Line: 2}, tpl.Access())

	tpl, err = parse(t, `<ssr:access role="*"/>`)
	require.NoError(t, err)
	assert.Equal(t, &Access{Roles: []string{"*"}, Line: 1}, tpl.Access())

	tpl, err = parse(t, `<ssr:access role="staff" shared="true"/>`)
	require.NoError(t, err)
	assert.Equal(t, &Access{Roles: []string{"staff"}, Shared: true, Line: 1}, tpl.Access())

	assert.NotContains(t, pageCode(t, `<p>a</p><ssr:access role="*"/>`), "access", "the tag renders nothing")

	_, err = parse(t, "<p>a</p>\n<ssr:access role=\"a\"/>\n<ssr:access role=\"b\"/>")
	var e *errs.Error
	require.ErrorAs(t, err, &e)
	assert.Equal(t, "pages/x/index.html:3", e.Pos)
	assert.Equal(t, "a second <ssr:access>; the first is at line 2", e.Msg)

	_, err = parse(t, `<ssr:access role=" "/>`)
	require.ErrorAs(t, err, &e)
	assert.Equal(t, "<ssr:access> has no role", e.Msg)
}

func TestAccessNotNested(t *testing.T) {
	for src, inside := range map[string]string{
		`<div ssr:if="a"><p><ssr:access role="x"/></p></div>`:                 "an element with ssr:if",
		`<p ssr:if="a"></p><div ssr:else-if="b"><ssr:access role="x"/></div>`: "an element with ssr:else-if",
		`<p ssr:if="a"></p><div ssr:else><ssr:access role="x"/></div>`:        "an element with ssr:else",
		`<ul><li ssr:for="i in items"><ssr:access role="x"/></li></ul>`:       "an element with ssr:for",
		`<ssr:form name="f"><div><ssr:access role="x"/></div></ssr:form>`:     "<ssr:form>",
	} {
		_, err := parse(t, src)
		var e *errs.Error
		if assert.ErrorAs(t, err, &e, src) {
			assert.Equal(t, "E-GEN-031", e.Code, src)
			assert.Equal(t, "<ssr:access> is inside "+inside+"; the rule always applies, so it must not be nested", e.Msg, src)
		}
	}

	for _, src := range []string{
		`<html><head><ssr:access role="x"/></head></html>`,
		`<div ssr:if="a"><p>a</p></div><div><ssr:access role="x"/></div>`,
		`<ssr:form name="f"></ssr:form><ssr:access role="x"/>`,
	} {
		tpl, err := parse(t, src)
		require.NoError(t, err, src)
		assert.NotNil(t, tpl.Access(), src)
	}
}

func TestCallTag(t *testing.T) {
	tpl, err := parse(t, `<ssr:call name="add" in="AddIn" out="AddOut"/><ssr:call name="ping" in="struct{}" out="string"/>`)
	require.NoError(t, err)
	assert.Equal(t, []Call{{Name: "add", In: "AddIn", Out: "AddOut", Line: 1}, {Name: "ping", In: "struct{}", Out: "string", Line: 1}}, tpl.Calls())

	for _, src := range []string{
		`<ssr:call in="A" out="B"/>`,
		`<ssr:call name="Add" in="A" out="B"/>`,
		`<ssr:call name="add-one" in="A" out="B"/>`,
		`<ssr:call name="add" out="B"/>`,
		`<ssr:call name="add" in="A"/>`,
		`<ssr:call name="add" in="A" out="B"/><ssr:call name="add" in="A" out="B"/>`,
	} {
		_, err := parse(t, src)
		assert.Equal(t, "E-GEN-040", errs.Code(err), src)
	}
}

func TestCallTagProblems(t *testing.T) {
	for src, msg := range map[string]string{
		`<ssr:call name="add" in=" " out="B"/>`:                                      `<ssr:call name="add"> has no in type`,
		`<ssr:call name="a" in="A" out="B" role="x"/>`:                               `<ssr:call> has the attribute role`,
		`<ssr:call name="a" in="A" out="B" ssr:if="x"/>`:                             `<ssr:call> has the attribute ssr:if`,
		`<ssr:call name="a" in="A" out="B"/><ssr:call name="a" in="C" out="D"/>`:     `a second <ssr:call name="a">; the first is at line 1`,
		`<div ssr:if="admin"><ssr:call name="a" in="A" out="B"/></div>`:              `<ssr:call name="a"> is inside an element with ssr:if; the page can always make the call, so it must not be nested`,
		`<ul><li ssr:for="i in items"><ssr:call name="a" in="A" out="B"/></li></ul>`: `<ssr:call name="a"> is inside an element with ssr:for; the page can always make the call, so it must not be nested`,
		`<ssr:form name="f"><ssr:call name="a" in="A" out="B"/></ssr:form>`:          `<ssr:call name="a"> is inside <ssr:form>; the page can always make the call, so it must not be nested`,
		`<ssr:call name="a" in="func()" out="B"/>`:                                   `<ssr:call name="a"> in: "func()" cannot be sent as JSON`,
		`<ssr:call name="a" in="A" out="chan int"/>`:                                 `<ssr:call name="a"> out: "chan int" cannot be sent as JSON`,
		`<ssr:call name="a" in="*(func(int) bool)" out="B"/>`:                        `<ssr:call name="a"> in: "*(func(int) bool)" cannot be sent as JSON`,
		`<ssr:call name="a" in="A" out="*<-chan Note"/>`:                             `<ssr:call name="a"> out: "*<-chan Note" cannot be sent as JSON`,
	} {
		_, err := parse(t, src)
		var e *errs.Error
		if assert.ErrorAs(t, err, &e, src) {
			assert.Equal(t, "E-GEN-040", e.Code, src)
			assert.Equal(t, msg, e.Msg, src)
		}
	}

	for src, msg := range map[string]string{
		`<ssr:call name="a" in="web.SafeHTML" out="B"/>`:    `<ssr:call name="a"> in: "web.SafeHTML" names a type from another package`,
		`<ssr:call name="a" in="*time.Time" out="B"/>`:      `<ssr:call name="a"> in: "*time.Time" names a type from another package`,
		`<ssr:call name="a" in="struct{ X int }" out="B"/>`: `<ssr:call name="a"> in: "struct{ X int }" is not a type name`,
	} {
		_, err := parse(t, src)
		var e *errs.Error
		if assert.ErrorAs(t, err, &e, src) {
			assert.Equal(t, "E-GEN-045", e.Code, src)
			assert.True(t, strings.HasPrefix(e.Msg, msg), "%s\n%s", src, e.Msg)
		}
	}

	for src, msg := range map[string]string{
		`<ssr:call name="a" in="args" out="B"/>`:          `<ssr:call name="a"> in: args is a name the generated code uses`,
		`<ssr:call name="a" in="A" out="[]in"/>`:          `<ssr:call name="a"> out: in is a name the generated code uses`,
		`<ssr:call name="a" in="map[string]r" out="B"/>`:  `<ssr:call name="a"> in: r is a name the generated code uses`,
		`<ssr:call name="a" in="A" out="[]func(x ctx)"/>`: `<ssr:call name="a"> out: ctx is a name the generated code uses`,
	} {
		_, err := parse(t, src)
		var e *errs.Error
		if assert.ErrorAs(t, err, &e, src) {
			assert.Equal(t, "E-GEN-042", e.Code, src)
			assert.Equal(t, msg, e.Msg, src)
		}
	}
	for _, src := range []string{`<ssr:call name="a" in="Args" out="[]func(name string)"/>`, `<ssr:call name="a" in="Page[Note]" out="B"/>`} {
		_, err := parse(t, src)
		require.NoError(t, err, src)
	}

	tpl, err := parse(t, `<head><ssr:call name="ping1" in="struct {}" out="[]*Note"/></head>`)
	require.NoError(t, err)
	assert.Equal(t, []Call{{Name: "ping1", In: "struct{}", Out: "[]*Note", Line: 1}}, tpl.Calls())
	assert.NotContains(t, pageCode(t, `<p>a</p><ssr:call name="a" in="A" out="B"/>`), "call", "the tag renders nothing")
}

func TestVarType(t *testing.T) {
	for src, want := range map[string]string{
		"string":                     "string",
		"[] Note":                    "[]Note",
		"*Note":                      "*Note",
		"map[string][4]int":          "map[string][4]int",
		"Page[Note, int]":            "Page[Note, int]",
		"func(string, ...int) bool":  "func(string, ...int) bool",
		"web.SafeHTML":               "web.SafeHTML",
		"map[string][]*web.SafeHTML": "map[string][]*web.SafeHTML",
	} {
		tpl, err := parse(t, `<ssr:var name="v" type="`+src+`"/>`)
		require.NoError(t, err, src)
		assert.Equal(t, want, tpl.GetVariables()[0].Type, src)
	}

	for _, src := range []string{
		"int; func init() {}",
		"int\nfunc init() {}",
		"f()",
		"func() int { return 1 }()",
		"struct{}",
		"chan int",
		"[n]int",
		"a.b.c",
		"_",
		"*time.Time",
		"[]Page[time.Time]",
		"func(time.Duration)",
		"web.safeHTML",
		"render.If",
		"[]form.Input[string]",
	} {
		_, err := parse(t, `<ssr:var name="v" type="`+src+`"/>`)
		assert.Equal(t, "E-GEN-045", errs.Code(err), src)
	}

	for src, used := range map[string]string{"msg": "msg", "[]conn": "conn", "map[string]ctx": "ctx", "*r": "r", "func(s)": "s"} {
		_, err := parse(t, `<ssr:var name="v" type="`+src+`"/>`)
		var e *errs.Error
		if assert.ErrorAs(t, err, &e, src) {
			assert.Equal(t, "E-GEN-042", e.Code, src)
			assert.Equal(t, `<ssr:var name="v"> type: `+used+" is a name the generated code uses", e.Msg, src)
		}
	}
	for _, src := range []string{"Msg", "func(msg string)", "Conn"} {
		_, err := parse(t, `<ssr:var name="v" type="`+src+`"/>`)
		require.NoError(t, err, src)
	}

	_, err := parse(t, `<ssr:var name="v" type="*time.Time"/>`)
	var e *errs.Error
	require.ErrorAs(t, err, &e)
	assert.Equal(t, `<ssr:var name="v">: "*time.Time" names a type from another package, which the generated code cannot import; `+
		"use a type of package web, such as web.SafeHTML, or declare a named type in a .go file next to the template", e.Msg)
	_, err = parse(t, `<ssr:var name="v"/>`)
	assert.Equal(t, "E-GEN-003", errs.Code(err), "a missing type keeps its own code")
}
