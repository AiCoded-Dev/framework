package generate

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
)

func readTree(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		rel, _ := filepath.Rel(root, p)
		files[filepath.ToSlash(rel)] = string(data)
		return err
	}))
	return files
}

func find(t *testing.T, files map[string]string, pattern string) string {
	t.Helper()
	re := regexp.MustCompile(pattern)
	for name, data := range files {
		if re.MatchString(name) {
			return data
		}
	}
	t.Fatalf("no file matches %s in %v", pattern, files)
	return ""
}

func TestAssets(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	dir := copyApp(t, "app")
	require.NoError(t, Generate(dir))
	files := readTree(t, filepath.Join(dir, "pages", "assets_gen"))

	css := find(t, files, `^pages/index-[A-Z0-9]+\.css$`)
	assert.Contains(t, css, "body p{", "nesting is lowered for older browsers")
	assert.Contains(t, css, "/_aicoded/assets/assets/icon-")
	assert.Contains(t, find(t, files, `^pages/notes/index-[A-Z0-9]+\.js$`), "toUpperCase")
	for name := range files {
		assert.NotRegexp(t, `\.map$`, name, "no source maps are built")
	}
	find(t, files, `^pages/notes/logo-[0-9a-f]{8}\.png$`)
	find(t, files, `^assets/icon-[A-Z0-9]+\.svg$`)

	root, err := os.ReadFile(filepath.Join(dir, "pages/route_gen.go"))
	require.NoError(t, err)
	assert.Regexp(t, `<link rel=\\"stylesheet\\" href=\\"/_aicoded/assets/pages/index-[A-Z0-9]+\.css\\">`, string(root))
	notes, err := os.ReadFile(filepath.Join(dir, "pages/notes/route_gen.go"))
	require.NoError(t, err)
	assert.Regexp(t, `<script defer src=\\"/_aicoded/assets/pages/notes/index-[A-Z0-9]+\.js\\"></script>`, string(notes))
	assert.Contains(t, string(notes), "/_aicoded/assets/pages/notes/logo-")
	embed, err := os.ReadFile(filepath.Join(dir, "pages/assets_gen.go"))
	require.NoError(t, err)
	assert.Contains(t, string(embed), "//go:embed all:assets_gen")
	goVet(t, dir)

	require.NoError(t, Generate(dir))
	assert.Equal(t, files, readTree(t, filepath.Join(dir, "pages", "assets_gen")), "builds are deterministic")
	other := copyApp(t, "app")
	require.NoError(t, Generate(other))
	assert.Equal(t, files, readTree(t, filepath.Join(other, "pages", "assets_gen")), "the app's folder does not change the build")
}

func TestAssetTags(t *testing.T) {
	g := built(t, map[string]string{
		"pages/index.html":   doc(`<ssr:access role="*"/><ssr:assets/><ssr:content/>`),
		"pages/index.ts":     `import "./extra.css"; console.log(1);`,
		"pages/extra.css":    `a { color: red; }`,
		"pages/index.css":    `b { color: blue; }`,
		"pages/a/index.html": `<p>a</p>`,
	})
	tags := g.assetTags("/")
	require.Len(t, tags, 3)
	assert.Regexp(t, `^<link rel="stylesheet" href="/_aicoded/assets/pages/index-[A-Z0-9]+\.css">$`, tags[0])
	assert.Regexp(t, `^<link rel="stylesheet" href="/_aicoded/assets/pages/index-[A-Z0-9]+\.css">$`, tags[1])
	assert.Regexp(t, `^<script defer src="/_aicoded/assets/pages/index-[A-Z0-9]+\.js"></script>$`, tags[2])
	assert.NotEqual(t, tags[0], tags[1], "the stylesheet the script imports and index.css")
	assert.Equal(t, []string{}, g.assetTags("/a"))
}

func TestImage(t *testing.T) {
	app := newApp(t, map[string]string{"pages/a/my logo.png": "png", "assets/i.svg": "svg"})
	a := newAssets(app)
	dir := filepath.Join(app.Dir, "pages", "a")
	for src, want := range map[string]string{
		"my logo.png":               "/_aicoded/assets/pages/a/my%20logo-8f8cbb7d.png",
		"/assets/i.svg":             "/_aicoded/assets/assets/i-acdb1373.svg",
		"../../assets/i.svg":        "/_aicoded/assets/assets/i-acdb1373.svg",
		"/notes/1":                  "/notes/1",
		"//cdn.example/x.png":       "//cdn.example/x.png",
		"data:image/png;base64,AAA": "data:image/png;base64,AAA",
	} {
		got, err := a.image(dir, src)
		require.NoError(t, err, src)
		assert.Equal(t, want, got, src)
	}
	assert.Equal(t, map[string][]byte{"pages/a/my logo-8f8cbb7d.png": []byte("png"), "assets/i-acdb1373.svg": []byte("svg")}, a.files)
}

func TestAssetsStayInTheApp(t *testing.T) {
	outside := t.TempDir()
	write(t, outside, "secret.json", `{"key": "x"}`)
	write(t, outside, "x.png", "png")
	link := func(t *testing.T, dir, rel, target string) {
		t.Helper()
		if err := os.Symlink(target, filepath.Join(dir, rel)); err != nil {
			t.Skip("cannot create symbolic links:", err)
		}
	}

	type problem struct {
		setup    func(t *testing.T, dir string)
		pos, msg string
	}
	for name, c := range map[string]problem{
		"absolute import": {func(t *testing.T, dir string) {
			write(t, dir, "pages/notes/index.ts", `import s from "`+filepath.ToSlash(filepath.Join(outside, "secret.json"))+`"; console.log(s);`)
		}, "pages/notes/index.ts:1:", "secret.json is outside the app's pages/ and assets/ folders"},
		"import through a link": {func(t *testing.T, dir string) {
			link(t, dir, "pages/notes/s.json", filepath.Join(outside, "secret.json"))
			write(t, dir, "pages/notes/index.ts", `import s from "./s.json"; console.log(s);`)
		}, "pages/notes/index.ts:1:", "secret.json is outside the app's pages/ and assets/ folders"},
		"import outside pages": {func(t *testing.T, dir string) {
			write(t, dir, "lib.ts", `export const x = 1;`)
			write(t, dir, "pages/notes/index.ts", `import { x } from "../../lib"; console.log(x);`)
		}, "pages/notes/index.ts:1:", ": lib.ts is outside the app's pages/ and assets/ folders"},
		"package import": {func(t *testing.T, dir string) {
			write(t, dir, "pages/notes/node_modules/lodash/index.js", `export default 1;`)
			write(t, dir, "pages/notes/index.ts", `import x from "lodash"; console.log(x);`)
		}, "pages/notes/index.ts:1:", `"lodash" is not a file of the app: scripts import only files of the app`},
		"path through node_modules": {func(t *testing.T, dir string) {
			write(t, dir, "pages/notes/node_modules/x/index.js", `export default 1;`)
			write(t, dir, "pages/notes/index.ts", `import x from "./node_modules/x/index.js"; console.log(x);`)
		}, "pages/notes/index.ts:1:", "pages/notes/node_modules/x/index.js is in a node_modules folder: scripts import only files of the app"},
		"style from a package": {func(t *testing.T, dir string) {
			write(t, dir, "pages/notes/node_modules/pkg/x.css", `a { color: red; }`)
			write(t, dir, "pages/notes/x.css", `@import "pkg/x.css";`)
			write(t, dir, "pages/notes/index.ts", `import "./x.css";`)
		}, "pages/notes/x.css:1:", "pages/notes/node_modules/pkg/x.css is in a node_modules folder"},
		"style image Go cannot embed": {func(t *testing.T, dir string) {
			write(t, dir, "pages/notes/a'b.svg", `<svg xmlns="http://www.w3.org/2000/svg"/>`)
			write(t, dir, "pages/notes/x.css", `a { background: url("a'b.svg"); }`)
			write(t, dir, "pages/notes/index.ts", `import "./x.css";`)
		}, "pages/notes/x.css:1:", "pages/notes/a'b.svg has a name Go cannot embed"},
	} {
		dir := copyApp(t, "app")
		c.setup(t, dir)
		err := Generate(dir)
		require.ErrorContains(t, err, c.pos, name)
		require.ErrorContains(t, err, c.msg, name)
		assert.Equal(t, "E-GEN-036", errs.Code(err), name)
	}

	for name, c := range map[string]problem{
		"image through a link": {func(t *testing.T, dir string) {
			link(t, dir, "pages/empty/x.png", filepath.Join(outside, "x.png"))
			write(t, dir, "pages/empty/index.html", `<img src="x.png" alt="">`)
		}, "pages/empty/index.html:1", "the image x.png is outside the app's pages/ and assets/ folders"},
		"image outside pages": {func(t *testing.T, dir string) {
			write(t, dir, "pages/empty/index.html", `<img src="../../go.mod" alt="">`)
		}, "pages/empty/index.html:1", "the image ../../go.mod is outside the app's pages/ and assets/ folders"},
		"folder as image": {func(t *testing.T, dir string) {
			write(t, dir, "pages/empty/index.html", `<img src="/assets/" alt="">`)
		}, "pages/empty/index.html:1", "the image /assets/ is not a file"},
		"source file as image": {func(t *testing.T, dir string) {
			write(t, dir, "pages/empty/index.html", `<img src="../notes/dataprovider.go" alt="">`)
		}, "pages/empty/index.html:1", "the image ../notes/dataprovider.go is not an image"},
		"name Go cannot embed": {func(t *testing.T, dir string) {
			write(t, dir, "pages/empty/a'b.png", "png")
			write(t, dir, "pages/empty/index.html", `<img src="a'b.png" alt="">`)
		}, "pages/empty/index.html:1", "the image a'b.png has a name Go cannot embed"},
		"folder Go cannot embed": {func(t *testing.T, dir string) {
			write(t, dir, "pages/empty/img's/x.png", "png")
			write(t, dir, "pages/empty/index.html", `<img src="img's/x.png" alt="">`)
		}, "pages/empty/index.html:1", "the image img's/x.png has a name Go cannot embed"},
	} {
		dir := copyApp(t, "app")
		c.setup(t, dir)
		err := Generate(dir)
		require.ErrorContains(t, err, c.pos+": E-GEN-035: "+c.msg, name)
	}
}

func TestEmbeddable(t *testing.T) {
	for _, p := range []string{"pages/a/logo-1a2b3c4d.png", "assets/my logo!#$%&()+,-=@[]^_{}~.png", "pages/.well/_x.svg", "pages/café.png"} {
		assert.True(t, embeddable(p), p)
	}
	for _, p := range []string{"", "a//b.png", "a/", "a/../b.png", "a/./b.png", "a'b.png", `a"b.png`, "a*b.png", "a<b.png", "a>b.png",
		"a?b.png", "a`b.png", "a|b.png", `a\b.png`, "a:b.png", "a★.png", "trail.", "CON.png", "pages/nul/x.png", "com1", "lpt9.svg",
		".git/x.png", "a/.hg/x.png", "a/.svn", ".bzr", "\xff.png"} {
		assert.False(t, embeddable(p), p)
	}
}

func TestImageIsEscapedOnce(t *testing.T) {
	dir := copyApp(t, "app")
	write(t, dir, "pages/empty/a&b.png", "png")
	write(t, dir, "pages/empty/index.html", `<img src="a&amp;b.png" alt="">`)
	require.NoError(t, Generate(dir))
	code, err := os.ReadFile(filepath.Join(dir, "pages/empty/route_gen.go"))
	require.NoError(t, err)
	assert.Contains(t, string(code), `<img src=\"/_aicoded/assets/pages/empty/a&amp;b-8f8cbb7d.png\" alt>`)
	assert.FileExists(t, filepath.Join(dir, "pages/assets_gen/pages/empty/a&b-8f8cbb7d.png"))
}

func TestLiveScriptImportsItsTypes(t *testing.T) {
	dir := copyApp(t, "app")
	write(t, dir, "pages/live/index.ts", `import { ssr } from "./reactive_gen"; ssr.on("count", (v) => console.log(v));`)
	require.NoFileExists(t, filepath.Join(dir, "pages/live/reactive_gen.ts"))
	require.NoError(t, Generate(dir), "the types are built from memory before they are written")
	js := find(t, readTree(t, filepath.Join(dir, "pages", "assets_gen")), `^pages/live/index-[A-Z0-9]+\.js$`)
	assert.Contains(t, js, `aicodedReactive.route("b82f9d7d")`)

	write(t, dir, "pages/live/reactive_gen.ts", "export const ssr = { on() {} };\n")
	require.NoError(t, Generate(dir))
	assert.Equal(t, js, find(t, readTree(t, filepath.Join(dir, "pages", "assets_gen")), `^pages/live/index-[A-Z0-9]+\.js$`),
		"an old copy on disk is never read")

	write(t, dir, "pages/empty/reactive_gen.ts", "export const ssr = 1;\n")
	write(t, dir, "pages/empty/index.ts", `import { ssr } from "./reactive_gen"; console.log(ssr);`)
	err := Generate(dir)
	require.ErrorContains(t, err, "pages/empty/index.ts:1:21: E-GEN-036: pages/empty/reactive_gen.ts is not generated in this run")
}

func TestAssetsIgnoreTsconfig(t *testing.T) {
	dir := copyApp(t, "app")
	for _, d := range []string{dir, filepath.Dir(dir)} {
		write(t, d, "tsconfig.json", `{"compilerOptions": {"alwaysStrict": true}}`)
	}
	require.NoError(t, Generate(dir))
	js := find(t, readTree(t, filepath.Join(dir, "pages", "assets_gen")), `^pages/notes/index-[A-Z0-9]+\.js$`)
	assert.NotContains(t, js, "use strict")
}

func TestAssetsAreReplaced(t *testing.T) {
	dir := copyApp(t, "app")
	require.NoError(t, Generate(dir))
	for _, p := range []string{"pages/index.css", "pages/notes/index.ts", "pages/live", "pages/tools"} {
		require.NoError(t, os.RemoveAll(filepath.Join(dir, p)))
	}
	write(t, dir, "pages/notes/index.html", `<p>no image</p><ssr:content/>`)
	require.NoError(t, Generate(dir))
	_, err := os.Stat(filepath.Join(dir, "pages", "assets_gen"))
	require.ErrorIs(t, err, fs.ErrNotExist, "no assets are left behind")
	embed, err := os.ReadFile(filepath.Join(dir, "pages/assets_gen.go"))
	require.NoError(t, err)
	assert.Equal(t, assetsCode, string(embed))
}

func TestAssetsNeverWriteThroughLinks(t *testing.T) {
	dir := copyApp(t, "app")
	outside := t.TempDir()
	write(t, outside, "keep.txt", "keep")
	if err := os.Symlink(outside, filepath.Join(dir, "pages", "assets_gen")); err != nil {
		t.Skip("cannot create symbolic links:", err)
	}
	require.NoError(t, Generate(dir))
	assert.Equal(t, map[string]string{"keep.txt": "keep"}, readTree(t, outside))
	st, err := os.Lstat(filepath.Join(dir, "pages", "assets_gen"))
	require.NoError(t, err)
	assert.True(t, st.IsDir(), "the link is replaced by the built assets")
}

func TestAssetErrors(t *testing.T) {
	dir := copyApp(t, "app")
	write(t, dir, "pages/notes/index.ts", "const x = ;\n")
	err := Generate(dir)
	require.ErrorContains(t, err, "pages/notes/index.ts:1:")
	assert.Equal(t, "E-GEN-036", errs.Code(err))

	dir = copyApp(t, "app")
	write(t, dir, "pages/index.css", `a { background: url("nope.png"); }`)
	err = Generate(dir)
	require.ErrorContains(t, err, `pages/index.css:1:21: E-GEN-036: Could not resolve "nope.png"`)

	for _, src := range []string{"missing.png", "../../../../etc/passwd", "/assets/missing.svg"} {
		dir = copyApp(t, "app")
		write(t, dir, "pages/empty/index.html", `<img src="`+src+`" alt="">`)
		err = Generate(dir)
		require.ErrorContains(t, err, "pages/empty/index.html:1", src)
		assert.Equal(t, "E-GEN-035", errs.Code(err), src)
	}

	dir = copyApp(t, "app")
	write(t, dir, "pages/empty/index.html", `<img src="https://cdn.example/x.png" alt=""><img src="{{ x }}" alt=""><ssr:var name="x" type="string"/>`)
	assert.NoError(t, Generate(dir), "external and computed images are left alone")
}

func TestLivePagesGetTheRuntime(t *testing.T) {
	dir := copyApp(t, "app")
	require.NoError(t, Generate(dir))
	files := readTree(t, filepath.Join(dir, "pages", "assets_gen"))
	js := find(t, files, `^aicoded/reactive-[A-Z0-9]+\.js$`)
	assert.Contains(t, js, "aicodedReactive")
	assert.Contains(t, js, "__ws")
	assert.Contains(t, find(t, files, `^aicoded/reactive-[A-Z0-9]+\.css$`), "display:contents")

	live, err := os.ReadFile(filepath.Join(dir, "pages/live/route_gen.go"))
	require.NoError(t, err)
	assert.Regexp(t, `SetAssets\(\[\]string\{\s*"<link rel=\\"stylesheet\\" href=\\"/_aicoded/assets/aicoded/reactive-`, string(live))
	notes, err := os.ReadFile(filepath.Join(dir, "pages/notes/route_gen.go"))
	require.NoError(t, err)
	assert.NotContains(t, string(notes), "aicoded/reactive-")
	tools, err := os.ReadFile(filepath.Join(dir, "pages/tools/route_gen.go"))
	require.NoError(t, err)
	assert.Contains(t, string(tools), "aicoded/reactive-", "a page with calls needs the runtime too")

	require.NoError(t, Generate(dir))
	assert.Equal(t, files, readTree(t, filepath.Join(dir, "pages", "assets_gen")))
}

func TestRuntimeComesFirst(t *testing.T) {
	g := built(t, map[string]string{
		"pages/index.html":   doc(`<ssr:access role="*"/><ssr:assets/><ssr:var name="n" type="int" reactive="true"/><p>{{ n }}</p><ssr:content/>`),
		"pages/index.ts":     `console.log(1);`,
		"pages/a/index.html": `<ssr:call name="ping" in="int" out="int"/><p>a</p>`,
		"pages/a/index.ts":   `console.log(2);`,
		"pages/b/index.html": `<p>b</p>`,
	})
	root := g.assetTags("/")
	require.Len(t, root, 3)
	assert.Regexp(t, `^<link rel="stylesheet" href="/_aicoded/assets/aicoded/reactive-[A-Z0-9]+\.css">$`, root[0])
	assert.Regexp(t, `^<script defer src="/_aicoded/assets/aicoded/reactive-[A-Z0-9]+\.js"></script>$`, root[1])
	assert.Regexp(t, `^<script defer src="/_aicoded/assets/pages/index-[A-Z0-9]+\.js"></script>$`, root[2])
	child := g.assetTags("/a")
	require.Len(t, child, 3)
	assert.Equal(t, root[:2], child[:2], "a live layout and a live child share one runtime")
	assert.Regexp(t, `^<script defer src="/_aicoded/assets/pages/a/index-[A-Z0-9]+\.js"></script>$`, child[2])
	assert.Equal(t, []string{}, g.assetTags("/b"))

	var runtime []string
	for name := range g.assets.files {
		if strings.HasPrefix(name, "aicoded/") {
			runtime = append(runtime, name)
		}
	}
	assert.Len(t, runtime, 2, "the app has one runtime")
}

func TestRuntimeImportsOnlyItsFiles(t *testing.T) {
	for imp, msg := range map[string]string{
		"lodash":      `"lodash" is not a file of the live-page runtime`,
		"../secret":   `"../secret" is not a file of the live-page runtime`,
		"/etc/passwd": `"/etc/passwd" is not a file of the live-page runtime`,
		"./../secret": "../secret.ts is not a file of the live-page runtime",
		"./missing":   "missing.ts is not a file of the live-page runtime",
		"./data.json": "data.json is not a script or style of the live-page runtime",
	} {
		src := fstest.MapFS{
			"reactive.ts": {Data: []byte(`import x from "` + imp + `"; console.log(x);`)},
			"data.json":   {Data: []byte(`{}`)},
		}
		_, err := newAssets(newApp(t, nil)).runtimeFrom(src)
		require.ErrorContains(t, err, msg, imp)
	}

	a := newAssets(newApp(t, nil))
	_, err := a.runtimeFrom(fstest.MapFS{"reactive.ts": {Data: []byte(`console.log(1);`)}})
	require.ErrorContains(t, err, "the live-page runtime did not build a script and a stylesheet")
}

func TestRuntimeReadsOnlyItsSources(t *testing.T) {
	build := func() map[string][]byte {
		a := newAssets(newApp(t, nil))
		_, err := a.runtime()
		require.NoError(t, err)
		return a.files
	}
	want := build()
	dir := t.TempDir()
	write(t, dir, "package.json", `{"type": "commonjs", "sideEffects": false}`)
	write(t, dir, "tsconfig.json", `{"compilerOptions": {"alwaysStrict": true}}`)
	t.Chdir(dir)
	t.Setenv("TMPDIR", dir)
	assert.Equal(t, want, build())
}
