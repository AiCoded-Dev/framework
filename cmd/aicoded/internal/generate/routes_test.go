package generate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/template"
	"aicoded.dev/framework/internal/errs"
)

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
}

func newApp(t *testing.T, files map[string]string) App {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/site\n\ngo 1.26.0\n")
	for p, c := range files {
		write(t, dir, p, c)
	}
	return App{Dir: dir, Module: "example.com/site"}
}

func noImages(_, src string) (string, error) { return src, nil }

func paths(rs []*Route) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Path)
	}
	return out
}

// diags returns the positions and codes of discover's diagnostics.
func diags(t *testing.T, files map[string]string) []string {
	t.Helper()
	rs, err := discover(newApp(t, files), noImages)
	assert.Nil(t, rs)
	var d Diagnostics
	require.ErrorAs(t, err, &d)
	var out []string
	for _, e := range d {
		out = append(out, e.Pos+" "+e.Code)
	}
	return out
}

func TestDiscover(t *testing.T) {
	app := newApp(t, map[string]string{
		"pages/index.html":                 `<ssr:access role="*"/><ssr:content/>`,
		"pages/notes/index.html":           `<ssr:access role="editor, admin"/><ssr:content/>`,
		"pages/notes/n_id/index.html":      `<ssr:access role="editor" guard="true"/>`,
		"pages/group/sub/index.html":       `<p>under a folder without a template</p>`,
		"pages/assets_gen/pages/x-1234.js": `x`,
	})
	rs, err := discover(app, noImages)
	require.NoError(t, err)
	assert.Equal(t, []string{"/", "/group/sub", "/notes", "/notes/n_id"}, paths(rs))
	assert.Equal(t, &template.Access{Roles: []string{"editor", "admin"}, Line: 1}, rs[2].Template.Access())
	assert.True(t, rs[3].Template.Access().Guard)
	assert.Equal(t, filepath.Join(app.Dir, "pages", "notes", "n_id"), rs[3].Dir)
}

func TestDiscoverWithoutPages(t *testing.T) {
	rs, err := discover(newApp(t, nil), noImages)
	require.NoError(t, err)
	assert.Empty(t, rs)
}

func TestDiscoverImages(t *testing.T) {
	app := newApp(t, map[string]string{"pages/a/index.html": `<ssr:access role="*"/><img src="x.png">`})
	var got []string
	_, err := discover(app, func(dir, src string) (string, error) {
		got = append(got, dir, src)
		return src, nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(app.Dir, "pages", "a"), "x.png"}, got)

	_, err = discover(app, func(string, string) (string, error) { return "", errors.New("disk") })
	require.EqualError(t, err, "pages/a/index.html:1: disk", "an I/O error stops the run")
}

func TestDiscoverErrors(t *testing.T) {
	cases := map[string]map[string]string{
		"E-GEN-016": {"pages/index.html": `<ssr:access role="*"/>`, "pages/__ws/index.html": `x`},
		"E-GEN-030": {"pages/notes/index.html": `<p>open</p>`},
		"E-GEN-031": {"pages/index.html": `<ssr:access role="Admin"/>`},
		"E-GEN-032": {
			"pages/index.html":        `<ssr:access role="*"/><ssr:content/>`,
			"pages/n/index.html":      `<p>n</p>`,
			"pages/n/dataprovider.go": "package n\n\nfunc (p *DP) Guard() error { return nil }\n",
		},
		"E-GEN-041": {
			"pages/index.html":        `<ssr:access role="*"/><ssr:content/>`,
			"pages/n_id/index.html":   `<ssr:access role="*" shared="true"/>`,
			"pages/s_slug/index.html": `<ssr:access role="*" shared="true"/>`,
		},
		"E-GEN-043": {"pages/index.html": `<ssr:access role="*"/><ssr:content/>`, "pages/my-page/index.html": `<p>x</p>`},
	}
	for want, files := range cases {
		_, err := discover(newApp(t, files), noImages)
		assert.Equal(t, want, errs.Code(err), want)
	}

	_, err := discover(newApp(t, map[string]string{
		"pages/a/index.html": `<p ssr:bogus="x"></p>`,
		"pages/b/index.html": `<p ssr:else></p>`,
	}), noImages)
	var d Diagnostics
	require.ErrorAs(t, err, &d)
	assert.Len(t, d, 2, "every broken template is reported, not only the first")
}

func TestReservedFolders(t *testing.T) {
	assert.Equal(t, []string{
		"pages/_aicoded/index.html:1 E-GEN-016",
		"pages/a/__ws/index.html:1 E-GEN-016",
		"pages/a/assets_gen/index.html:1 E-GEN-016",
	}, diags(t, map[string]string{
		"pages/index.html":              `<ssr:access role="*"/>`,
		"pages/_aicoded/x.txt":          `x`,
		"pages/a/__ws/index.html":       `x`,
		"pages/a/__ws/b/index.html":     `<p>not walked</p>`,
		"pages/a/assets_gen/index.html": `x`,
	}))
}

func TestDenyByDefault(t *testing.T) {
	assert.Equal(t, []string{
		"pages/a/b/index.html:1 E-GEN-030",
		"pages/a/index.html:1 E-GEN-030",
		"pages/index.html:1 E-GEN-030",
	}, diags(t, map[string]string{
		"pages/index.html":       `<p>root</p><ssr:content/>`,
		"pages/a/index.html":     `<p>a</p><ssr:content/>`,
		"pages/a/b/index.html":   `<p>b</p>`,
		"pages/c/index.html":     `<ssr:access role="c"/><ssr:content/>`,
		"pages/c/d/e/index.html": `<p>e is under c</p>`,
	}))

	assert.Equal(t, []string{"pages/a/index.html:1 E-GEN-005"}, diags(t, map[string]string{
		"pages/a/index.html":   `<ssr:access role="*"/><p ssr:bogus="x"></p>`,
		"pages/a/b/index.html": `<p>b</p>`,
	}), "a route under a broken template is not reported as open")
}

func TestGuardDeclared(t *testing.T) {
	guard := "package n\n\nfunc (p *DP) Guard() error { return nil }\n"
	assert.Equal(t, []string{
		"pages/a/dp.go:3 E-GEN-032",
		"pages/b/dp.go:3 E-GEN-032",
		"pages/f/index.html:1 E-GEN-005",
	}, diags(t, map[string]string{
		"pages/index.html":            `<ssr:access role="*"/><ssr:content/>`,
		"pages/a/index.html":          `<ssr:access role="a"/>`,
		"pages/a/dp.go":               guard,
		"pages/b/index.html":          `<p>b</p>`,
		"pages/b/dp.go":               guard,
		"pages/c/index.html":          `<ssr:access role="c" guard="true"/>`,
		"pages/c/dp.go":               guard,
		"pages/d/index.html":          `<p>d</p>`,
		"pages/d/dp_gen.go":           guard,
		"pages/d/dp_test.go":          guard,
		"pages/d/func.go":             "package n\n\nfunc Guard() error { return nil }\n",
		"pages/e/dataprovider.go":     guard,
		"pages/f/index.html":          `<p ssr:bogus="x"></p>`,
		"pages/f/dataprovider.go":     guard,
		"pages/g/index.html":          `<ssr:access role="g"/>`,
		"pages/g/sub/dataprovider.go": guard,
	}))
}

// ruleChain returns templates from pages/ down, pages/index.html, pages/a/index.html and so on,
// each with an <ssr:access> on line 2 that has the attributes of one of rules.
func ruleChain(rules ...string) map[string]string {
	files := map[string]string{}
	dir := "pages"
	for i, attrs := range rules {
		files[dir+"/index.html"] = "<p>x</p>\n<ssr:access " + attrs + "/><ssr:content/>"
		dir += "/" + string(rune('a'+i))
	}
	return files
}

func TestAccessThatAddsNothing(t *testing.T) {
	const list = "a list admits anyone with one of its roles"
	for _, c := range []struct {
		rules         []string
		pos, msg, fix string
	}{
		{[]string{`role="staff"`, `role="staff,people"`}, "pages/a/index.html:2",
			`role="staff,people" adds nothing to role="staff" in pages/index.html: ` + list,
			`list only the roles this page adds, such as role="people"`},
		{[]string{`role="staff"`, `role="staff,people" guard="true"`}, "pages/a/index.html:2",
			`role="staff,people" adds nothing to role="staff" in pages/index.html: ` + list,
			`list only the roles this page adds, such as role="people" guard="true"`},
		{[]string{`role="staff"`, `role="*"`}, "pages/a/index.html:2",
			`role="*" adds nothing to role="staff" in pages/index.html: every rule on the path applies`,
			`remove this <ssr:access>: the rule in pages/index.html already decides who may open the page`},
		{[]string{`role="staff,hr"`, `role="staff,hr"`}, "pages/a/index.html:2",
			`role="staff,hr" adds nothing to role="staff,hr" in pages/index.html: ` + list,
			`remove this <ssr:access>: the rule in pages/index.html already admits these roles`},
		{[]string{`role="staff,hr"`, `role="staff,hr" guard="true"`}, "pages/a/index.html:2",
			`role="staff,hr" adds nothing to role="staff,hr" in pages/index.html: ` + list,
			`keep one role with the guard, such as role="staff" guard="true"`},
		{[]string{`role="staff,hr"`, `role="hr,staff" guard="true"`}, "pages/a/index.html:2",
			`role="hr,staff" adds nothing to role="staff,hr" in pages/index.html: ` + list,
			`keep one role with the guard, such as role="staff" guard="true"`},
		{[]string{`role="staff"`, `role="hr"`, `role="staff,people"`}, "pages/a/b/index.html:2",
			`role="staff,people" adds nothing to role="staff" in pages/index.html: ` + list,
			`list only the roles this page adds, such as role="people"`},
		{[]string{`role="staff"`, `role="hr"`, `role="staff,hr"`}, "pages/a/b/index.html:2",
			`role="staff,hr" adds nothing to role="hr" in pages/a/index.html: ` + list,
			`list only the roles this page adds, such as role="staff"`},
	} {
		_, err := discover(newApp(t, ruleChain(c.rules...)), noImages)
		var d Diagnostics
		if assert.ErrorAs(t, err, &d, c.rules) && assert.Len(t, d, 1, c.rules) {
			assert.Equal(t, []string{c.pos, "E-GEN-053", c.msg, c.fix}, []string{d[0].Pos, d[0].Code, d[0].Msg, d[0].Fix})
		}
	}
}

func TestAccessThatAddsSomething(t *testing.T) {
	for _, rules := range [][]string{
		{`role="staff"`, `role="staff"`},
		{`role="staff"`, `role="staff,staff"`},
		{`role="*"`, `role="*"`},
		{`role="staff"`, `role="staff" guard="true"`},
		{`role="staff"`, `role="*" guard="true"`},
		{`role="staff"`, `role="people"`},
		{`role="finance,auditor"`, `role="finance"`},
		{`role="*"`, `role="finance,auditor"`},
	} {
		_, err := discover(newApp(t, ruleChain(rules...)), noImages)
		assert.NoError(t, err, rules)
	}
}

func TestWhoseRecords(t *testing.T) {
	files := map[string]string{
		"pages/index.html":            `<ssr:access role="staff"/><ssr:content/>`,
		"pages/a/n_id/index.html":     "<p>a</p>\n<ssr:access role=\"staff\"/>",
		"pages/b/s_x/index.html":      `<ssr:content/>`,
		"pages/b/s_x/c/index.html":    `<p>c</p>`,
		"pages/d/s_x/index.html":      `<ssr:access role="staff" shared="true"/><ssr:content/>`,
		"pages/d/s_x/e/index.html":    `<p>the layout says it</p>`,
		"pages/f/n_id/index.html":     `<ssr:access role="staff" guard="true"/><ssr:content/>`,
		"pages/f/n_id/g/index.html":   `<p>the layout says it</p>`,
		"pages/h/n_id/i/index.html":   `<ssr:access role="staff" shared="true"/>`,
		"pages/j/index.html":          `<ssr:access role="staff" guard="true"/><ssr:content/>`,
		"pages/j/n_id/index.html":     `<p>a Guard above the parameter does not say it</p>`,
		"pages/k/n_a/index.html":      `<ssr:access role="staff" shared="true"/><ssr:content/>`,
		"pages/k/n_a/s_b/index.html":  `<p>only the deepest parameter counts</p>`,
		"pages/m/n_id/index.html":     `<p ssr:bogus="x"></p>`,
		"pages/m/n_id/x/index.html":   `<p>below a template that did not parse</p>`,
		"pages/n/n_id/o/index.html":   `<p>no template at the parameter</p>`,
		"pages/n/n_id/o/p/index.html": `<ssr:access role="staff" guard="true"/>`,
	}
	assert.Equal(t, []string{
		"pages/a/n_id/index.html:2 E-GEN-054",
		"pages/b/s_x/c/index.html:1 E-GEN-054",
		"pages/b/s_x/index.html:1 E-GEN-054",
		"pages/j/n_id/index.html:1 E-GEN-054",
		"pages/k/n_a/s_b/index.html:1 E-GEN-054",
		"pages/m/n_id/index.html:1 E-GEN-005",
		"pages/n/n_id/o/index.html:1 E-GEN-054",
	}, diags(t, files))

	_, err := discover(newApp(t, files), noImages)
	var d Diagnostics
	require.ErrorAs(t, err, &d)
	assert.Equal(t, "/a/n_id has a parameter in its URL but does not say whose records it shows", d[0].Msg)
	assert.Equal(t, "/b/s_x/c does not say whose records it shows, and no template between it and the parameter folder pages/b/s_x does", d[1].Msg)
	assert.Equal(t, template.Fix("E-GEN-054"), d[0].Fix)
}

func TestSharedWithoutMeaning(t *testing.T) {
	const decl = "\n<ssr:access role=\"staff\" %s/>"
	for _, c := range []struct {
		files         map[string]string
		pos, msg, fix string
	}{
		{map[string]string{"pages/a/n_id/index.html": fmt.Sprintf(decl, `guard="true" shared="true"`)}, "pages/a/n_id/index.html:2",
			`/a/n_id declares both guard="true" and shared="true"`,
			`keep one: guard="true" when the Guard decides who sees each record, or shared="true"`},
		{map[string]string{"pages/a/index.html": fmt.Sprintf(decl, `shared="true"`)}, "pages/a/index.html:2",
			`/a declares shared="true" but has no parameter in its URL`,
			`remove shared="true": only a page with an id in its URL shows one record of many`},
		{map[string]string{
			"pages/a/n_id/index.html":   `<ssr:access role="staff" guard="true"/><ssr:content/>`,
			"pages/a/n_id/b/index.html": fmt.Sprintf(decl, `shared="true"`),
		}, "pages/a/n_id/b/index.html:2", `/a/n_id/b declares shared="true" below the Guard of /a/n_id`,
			`remove shared="true": the Guard above still decides who sees each record`},
		{map[string]string{
			"pages/a/index.html":      `<ssr:access role="staff" guard="true"/><ssr:content/>`,
			"pages/a/n_id/index.html": fmt.Sprintf(decl, `shared="true"`),
		}, "pages/a/n_id/index.html:2", `/a/n_id declares shared="true" below the Guard of /a`,
			`remove shared="true": the Guard above still decides who sees each record`},
	} {
		c.files["pages/index.html"] = `<ssr:access role="staff"/><ssr:content/>`
		_, err := discover(newApp(t, c.files), noImages)
		var d Diagnostics
		if assert.ErrorAs(t, err, &d, c.pos) && assert.Len(t, d, 1, c.pos) {
			assert.Equal(t, []string{c.pos, "E-GEN-055", c.msg, c.fix}, []string{d[0].Pos, d[0].Code, d[0].Msg, d[0].Fix})
		}
	}

	_, err := discover(newApp(t, map[string]string{
		"pages/index.html":             `<ssr:access role="staff,hr"/><ssr:content/>`,
		"pages/a/s_x/index.html":       `<ssr:access role="*" shared="true"/><ssr:content/>`,
		"pages/a/s_x/b/index.html":     `<ssr:access role="staff" guard="true"/>`,
		"pages/a/s_x/b/c/index.html":   `<ssr:access role="hr"/>`,
		"pages/a/s_x/d/n_y/index.html": `<ssr:access role="staff" shared="true"/>`,
	}), noImages)
	assert.NoError(t, err, `a Guard below shared="true" narrows it, and role="*" with shared="true" keeps the rule above`)
}

func TestSharedAddsNothing(t *testing.T) {
	for _, c := range []struct{ above, rule, fix string }{
		{`role="staff"`, `role="staff,people" shared="true"`, `list only the roles this page adds, such as role="people" shared="true"`},
		{`role="staff,hr"`, `role="hr,staff" shared="true"`, `write role="*" shared="true": the rule in pages/index.html already admits these roles`},
	} {
		_, err := discover(newApp(t, map[string]string{
			"pages/index.html":      "<ssr:access " + c.above + "/><ssr:content/>",
			"pages/n_id/index.html": "<ssr:access " + c.rule + "/>",
		}), noImages)
		var d Diagnostics
		if assert.ErrorAs(t, err, &d, c.rule) && assert.Len(t, d, 1, c.rule) {
			assert.Equal(t, []string{"E-GEN-053", c.fix}, []string{d[0].Code, d[0].Fix})
		}
	}
}

func TestFolderNames(t *testing.T) {
	assert.Equal(t, []string{
		"pages/.cache E-GEN-043",
		"pages/1st/index.html:1 E-GEN-043",
		"pages/_/index.html:1 E-GEN-043",
		"pages/_partials E-GEN-043",
		"pages/main/index.html:1 E-GEN-043",
		"pages/my-group E-GEN-043",
		"pages/n_/index.html:1 E-GEN-043",
		"pages/ok/_old/index.html:1 E-GEN-043",
		"pages/testdata/index.html:1 E-GEN-043",
		"pages/type/index.html:1 E-GEN-043",
		"pages/x;func init(){}/index.html:1 E-GEN-043",
		"pages/Ünï_2/index.html:1 E-GEN-043",
	}, diags(t, map[string]string{
		"pages/index.html":                 `<ssr:access role="*"/><ssr:content/>`,
		"pages/.cache/x.go":                "package cache\n",
		"pages/1st/index.html":             `<p>1</p>`,
		"pages/_/index.html":               `<p>_</p>`,
		"pages/_partials/x.go":             "package partials\n",
		"pages/main/index.html":            `<p>main</p>`,
		"pages/my-group/a/index.html":      `<p>a</p>`,
		"pages/my-group/b/index.html":      `<p>b</p>`,
		"pages/n_/index.html":              `<p>n_</p>`,
		"pages/ok/_old/index.html":         `<p>old</p>`,
		"pages/testdata/index.html":        `<p>testdata</p>`,
		"pages/type/index.html":            `<p>type</p>`,
		"pages/x;func init(){}/index.html": `<p>x</p>`,
		"pages/Ünï_2/index.html":           `<p>Ünï</p>`,
		"pages/s_name/Ok_2/index.html":     `<ssr:access role="*" shared="true"/>`,
		"pages/no-page/a.png":              `png`,
	}))
}

// A page with pages below it and no <ssr:content/> is a gate, not an error.
func TestGates(t *testing.T) {
	rs, err := discover(newApp(t, map[string]string{
		"pages/index.html":       `<ssr:access role="*"/>`,
		"pages/a/index.html":     `<p>a</p>`,
		"pages/a/x/y/index.html": `<p>deep below a, with no page at a/x</p>`,
		"pages/b/index.html":     `<ssr:content default="c"/>`,
		"pages/b/c/index.html":   `<p>c</p>`,
	}), noImages)
	require.NoError(t, err)
	assert.Equal(t, []string{"/", "/a", "/a/x/y", "/b", "/b/c"}, paths(rs))
}

// A default page must be a page below the layout whose folders all have fixed names.
func TestDefaultPage(t *testing.T) {
	files := map[string]string{
		"pages/index.html":                `<ssr:access role="*"/><ssr:content default="/good"/>`,
		"pages/good/index.html":           `<p>good</p>`,
		"pages/rel/index.html":            `<ssr:content default="grp/sub/"/>`,
		"pages/rel/grp/sub/index.html":    `<p>below a folder without a page</p>`,
		"pages/item/n_id/index.html":      `<ssr:access role="*" shared="true"/><ssr:content default="info"/>`,
		"pages/item/n_id/info/index.html": `<p>below a parameter folder</p>`,
		"pages/missing/index.html":        "\n<ssr:content default=\"nothing\"/>",
		"pages/missing/x/index.html":      `<p>x</p>`,
		"pages/up/index.html":             "\n<ssr:content default=\"../good\"/>",
		"pages/up/x/index.html":           `<p>x</p>`,
		"pages/self/index.html":           "\n<ssr:content default=\".\"/>",
		"pages/self/x/index.html":         `<p>x</p>`,
		"pages/param/index.html":          "\n<ssr:content default=\"s_slug\"/>",
		"pages/param/s_slug/index.html":   `<ssr:access role="*" shared="true"/>`,
		"pages/query/index.html":          "\n<ssr:content default=\"x?tab=1\"/>",
		"pages/query/x/index.html":        `<p>x</p>`,
	}
	assert.Equal(t, []string{
		"pages/missing/index.html:2 E-GEN-049",
		"pages/param/index.html:2 E-GEN-049",
		"pages/query/index.html:2 E-GEN-049",
		"pages/self/index.html:2 E-GEN-049",
		"pages/up/index.html:2 E-GEN-049",
	}, diags(t, files))

	_, err := discover(newApp(t, files), noImages)
	var d Diagnostics
	require.ErrorAs(t, err, &d)
	assert.Equal(t, `<ssr:content default="nothing"> names /missing/nothing, where there is no page`, d[0].Msg)
	assert.Equal(t, `<ssr:content default="s_slug"> names the parameter folder s_slug, whose value is not known here; `+
		`name the page in DefaultRoute instead`, d[1].Msg)
	assert.Equal(t, `<ssr:content default="../good"> names /good, which is not below /up`, d[4].Msg)
}

func TestTwoParameterFolders(t *testing.T) {
	const shared = `<ssr:access role="*" shared="true"/>`
	files := map[string]string{
		"pages/index.html":            `<ssr:access role="*"/><ssr:content/>`,
		"pages/n/n_a/index.html":      shared,
		"pages/n/s_b/x/index.html":    shared,
		"pages/n/s_slug/index.html":   `<p ssr:bogus="x"></p>`,
		"pages/n/n_d/assets/a.png":    `png`,
		"pages/m/n_id/a/index.html":   shared,
		"pages/m/n_id/b/index.html":   shared,
		"pages/m/n_id/s_x/index.html": shared,
		"pages/k/n_id/index.html":     shared,
		"pages/k/s_id/index.html":     shared,
	}
	assert.Equal(t, []string{
		"pages/k/s_id/index.html:1 E-GEN-041",
		"pages/n/s_b E-GEN-041",
		"pages/n/s_slug/index.html:1 E-GEN-005",
		"pages/n/s_slug/index.html:1 E-GEN-041",
	}, diags(t, files))

	_, err := discover(newApp(t, files), noImages)
	var d Diagnostics
	require.ErrorAs(t, err, &d)
	assert.Equal(t, "pages/k has two parameter folders, n_id and s_id", d[0].Msg)
	assert.Equal(t, "pages/n has two parameter folders, n_a and s_b", d[1].Msg)
}
