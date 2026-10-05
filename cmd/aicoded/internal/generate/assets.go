package generate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/evanw/esbuild/pkg/api"

	"aicoded.dev/framework/web/reactive/client"
)

const (
	assetsDir  = "assets_gen"
	publicPath = "/_aicoded/assets/"
)

var (
	targets = []api.Engine{
		{Name: api.EngineChrome, Version: "111"},
		{Name: api.EngineFirefox, Version: "115"},
		{Name: api.EngineSafari, Version: "16.4"},
	}
	fileLoaders = map[string]api.Loader{
		".png": api.LoaderFile, ".jpg": api.LoaderFile, ".jpeg": api.LoaderFile, ".gif": api.LoaderFile,
		".svg": api.LoaderFile, ".webp": api.LoaderFile, ".ico": api.LoaderFile,
		".woff": api.LoaderFile, ".woff2": api.LoaderFile,
	}
)

// assets holds the built files, by path under pages/assets_gen, and the tags each route's
// <ssr:assets/> writes, by route path.
type assets struct {
	app App
	// root is the app's folder with symbolic links resolved.
	root  string
	files map[string][]byte
	tags  map[string][]string
	// generated holds the TypeScript files of this run, by path relative to the app. Scripts
	// import them from memory, never from disk.
	generated map[string][]byte
	// warnings holds the warnings of every build, each "<file>:<line>: <text>".
	warnings []string
}

func newAssets(app App) *assets {
	root, err := filepath.EvalSymlinks(app.Dir)
	if err != nil {
		root = app.Dir
	}
	return &assets{app: app, root: root, files: map[string][]byte{}, tags: map[string][]string{}}
}

func linkTag(href string) string {
	return `<link rel="stylesheet" href="` + html.EscapeString(href) + `">`
}

func scriptTag(src string) string {
	return `<script defer src="` + html.EscapeString(src) + `"></script>`
}

// image returns the served URL of an <img src> in the template in routeDir. A path next to the
// template or /assets/… is copied with a content hash in its name; the file must lie in the
// app's pages/ or assets/ folder. External, computed and page paths are left alone.
func (a *assets) image(routeDir, src string) (string, error) {
	switch {
	case src == "", strings.Contains(src, "{{"), strings.HasPrefix(src, "//"), strings.Contains(src, ":"):
		return src, nil
	}
	var p string
	if rest, ok := strings.CutPrefix(src, "/assets/"); ok {
		p = filepath.Join(a.app.Dir, "assets", filepath.FromSlash(rest))
	} else if strings.HasPrefix(src, "/") {
		return src, nil
	} else {
		p = filepath.Join(routeDir, filepath.FromSlash(src))
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", diag("", "E-GEN-035", "the image %s is not found", src)
	}
	rel, ok := a.local(resolved)
	if !ok {
		return "", diag("", "E-GEN-035", "the image %s is outside the app's pages/ and assets/ folders", src)
	}
	if st, err := os.Stat(resolved); err != nil || !st.Mode().IsRegular() {
		return "", diag("", "E-GEN-035", "the image %s is not a file", src)
	}
	ext := path.Ext(rel)
	if _, ok := fileLoaders[ext]; !ok {
		return "", diag("", "E-GEN-035", "the image %s is not an image: use a %s file", src, strings.Join(slices.Sorted(maps.Keys(fileLoaders)), ", "))
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	name := strings.TrimSuffix(rel, ext) + "-" + hex.EncodeToString(sum[:4]) + ext
	if !embeddable(name) {
		return "", diag("", "E-GEN-035", "the image %s has a name Go cannot embed: %s", src, embedRule)
	}
	a.files[name] = data
	return publicPath + (&url.URL{Path: name}).EscapedPath(), nil
}

// local returns the path of the file at resolved, a path without symbolic links, relative to
// the app, when it lies in the app's pages/ or assets/ folder: the only folders assets come from.
func (a *assets) local(resolved string) (string, bool) {
	for _, dir := range []string{"pages", "assets"} {
		if rel, err := filepath.Rel(filepath.Join(a.root, dir), resolved); err == nil && filepath.IsLocal(rel) {
			return path.Join(dir, filepath.ToSlash(rel)), true
		}
	}
	return "", false
}

// build bundles each route's index.ts and index.css. generated holds the TypeScript files this
// run generates, by path relative to the app; scripts import them from memory.
func (a *assets) build(routes []*Route, generated map[string][]byte) error {
	a.generated = map[string][]byte{}
	for p, data := range generated {
		if path.Ext(p) == ".ts" {
			a.generated[p] = data
		}
	}
	var entries []string
	routeOf := map[string]string{}
	for _, r := range routes {
		for _, name := range []string{"index.ts", "index.css"} {
			if _, err := os.Stat(filepath.Join(r.Dir, name)); err == nil {
				rel, err := filepath.Rel(a.app.Dir, filepath.Join(r.Dir, name))
				if err != nil {
					return err
				}
				entry := filepath.ToSlash(rel)
				entries = append(entries, entry)
				routeOf[entry] = r.Path
			}
		}
	}
	if len(entries) == 0 {
		return nil
	}
	outdir := filepath.Join(a.app.Dir, "pages", assetsDir)
	res := api.Build(api.BuildOptions{
		AbsWorkingDir:     a.app.Dir,
		EntryPoints:       entries,
		Bundle:            true,
		Write:             false,
		Outdir:            outdir,
		Outbase:           a.app.Dir,
		EntryNames:        "[dir]/[name]-[hash]",
		AssetNames:        "[dir]/[name]-[hash]",
		PublicPath:        publicPath,
		Format:            api.FormatIIFE,
		Target:            api.ES2020,
		Engines:           targets,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		MinifySyntax:      true,
		LegalComments:     api.LegalCommentsNone,
		Charset:           api.CharsetUTF8,
		Loader:            fileLoaders,
		TsconfigRaw:       "{}",
		Metafile:          true,
		LogLevel:          api.LogLevelSilent,
		Plugins:           []api.Plugin{a.confine()},
	})
	a.warnings = append(a.warnings, esbuildWarnings(res.Warnings)...)
	if len(res.Errors) > 0 {
		return buildErrors(res.Errors)
	}
	for _, f := range res.OutputFiles {
		rel, err := filepath.Rel(outdir, f.Path)
		if err != nil {
			return err
		}
		a.files[filepath.ToSlash(rel)] = f.Contents
	}
	var meta struct {
		Outputs map[string]struct {
			EntryPoint string `json:"entryPoint"`
			CSSBundle  string `json:"cssBundle"`
		} `json:"outputs"`
	}
	if err := json.Unmarshal([]byte(res.Metafile), &meta); err != nil {
		return err
	}
	prefix := "pages/" + assetsDir + "/"
	for out, o := range meta.Outputs {
		route, ok := routeOf[o.EntryPoint]
		if !ok {
			continue
		}
		switch path.Ext(out) {
		case ".js":
			a.tags[route] = append(a.tags[route], scriptTag(publicPath+strings.TrimPrefix(out, prefix)))
			if o.CSSBundle != "" {
				a.tags[route] = append(a.tags[route], linkTag(publicPath+strings.TrimPrefix(o.CSSBundle, prefix)))
			}
		case ".css":
			a.tags[route] = append(a.tags[route], linkTag(publicPath+strings.TrimPrefix(out, prefix)))
		}
	}
	for _, tags := range a.tags {
		slices.SortFunc(tags, func(x, y string) int { // stylesheets first, then scripts
			if xs, ys := strings.HasPrefix(x, "<script"), strings.HasPrefix(y, "<script"); xs != ys {
				if xs {
					return 1
				}
				return -1
			}
			return strings.Compare(x, y)
		})
	}
	return nil
}

// runtime bundles the live-page runtime and returns its tags.
func (a *assets) runtime() ([]string, error) {
	return a.runtimeFrom(client.Files)
}

// runtimeFrom bundles the live-page runtime whose sources are src. The build reads nothing but
// src and writes no source map, so its output depends on the sources alone.
func (a *assets) runtimeFrom(src fs.FS) ([]string, error) {
	res := api.Build(api.BuildOptions{
		EntryPoints:       []string{client.Entry},
		Bundle:            true,
		Write:             false,
		Outdir:            filepath.Join(a.app.Dir, "pages", assetsDir, "aicoded"),
		EntryNames:        "[name]-[hash]",
		Format:            api.FormatIIFE,
		Target:            api.ES2020,
		Engines:           targets,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		MinifySyntax:      true,
		LegalComments:     api.LegalCommentsNone,
		Charset:           api.CharsetUTF8,
		TsconfigRaw:       "{}",
		LogLevel:          api.LogLevelSilent,
		Plugins:           []api.Plugin{fromFS(src)},
	})
	a.warnings = append(a.warnings, esbuildWarnings(res.Warnings)...)
	if len(res.Errors) > 0 {
		return nil, buildErrors(res.Errors)
	}
	var css, js string
	for _, f := range res.OutputFiles {
		name := "aicoded/" + filepath.Base(f.Path)
		a.files[name] = f.Contents
		switch path.Ext(name) {
		case ".css":
			css = linkTag(publicPath + name)
		case ".js":
			js = scriptTag(publicPath + name)
		}
	}
	if css == "" || js == "" {
		return nil, errors.New("the live-page runtime did not build a script and a stylesheet: the aicoded framework is broken")
	}
	return []string{css, js}, nil
}

// runtimeNamespace is the esbuild namespace of the files of the live-page runtime.
const runtimeNamespace = "aicoded-runtime"

// fromFS serves a build from the .ts and .css files in src alone: every import is a path of a
// file in src that starts with "./", and a path without an extension names a .ts file.
func fromFS(src fs.FS) api.Plugin {
	loaders := map[string]api.Loader{".ts": api.LoaderTS, ".css": api.LoaderCSS}
	return api.Plugin{Name: runtimeNamespace, Setup: func(b api.PluginBuild) {
		b.OnResolve(api.OnResolveOptions{Filter: ".*"}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
			p := args.Path
			if args.Kind != api.ResolveEntryPoint {
				rel, ok := strings.CutPrefix(p, "./")
				if !ok {
					return api.OnResolveResult{}, fmt.Errorf("%q is not a file of the live-page runtime", p)
				}
				p = path.Join(path.Dir(args.Importer), rel)
			}
			if path.Ext(p) == "" {
				p += ".ts"
			}
			if _, ok := loaders[path.Ext(p)]; !ok {
				return api.OnResolveResult{}, fmt.Errorf("%s is not a script or style of the live-page runtime", p)
			}
			if st, err := fs.Stat(src, p); err != nil || !st.Mode().IsRegular() {
				return api.OnResolveResult{}, fmt.Errorf("%s is not a file of the live-page runtime", p)
			}
			return api.OnResolveResult{Path: p, Namespace: runtimeNamespace}, nil
		})
		b.OnLoad(api.OnLoadOptions{Filter: ".*", Namespace: runtimeNamespace}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {
			data, err := fs.ReadFile(src, args.Path)
			if err != nil {
				return api.OnLoadResult{}, err
			}
			contents := string(data)
			return api.OnLoadResult{Contents: &contents, Loader: loaders[path.Ext(args.Path)]}, nil
		})
	}}
}

// confine keeps scripts and styles to files of the app. It refuses imports of packages, files
// outside the app's pages/ and assets/ folders, whether named by path or reached through a
// symbolic link, files in a node_modules folder, files Go cannot embed that the build would
// copy, and a reactive_gen.ts this run does not generate. It serves the TypeScript files of
// this run from memory.
func (a *assets) confine() api.Plugin {
	return api.Plugin{Name: "aicoded", Setup: func(b api.PluginBuild) {
		b.OnResolve(api.OnResolveOptions{Filter: ".*"}, a.resolve)
		b.OnLoad(api.OnLoadOptions{Filter: ".*", Namespace: "file"}, a.load)
	}}
}

// resolve refuses a script import that is not a path, and points an import of a generated
// file at its content in memory.
func (a *assets) resolve(args api.OnResolveArgs) (api.OnResolveResult, error) {
	switch args.Kind {
	case api.ResolveEntryPoint, api.ResolveCSSImportRule, api.ResolveCSSComposesFrom, api.ResolveCSSURLToken:
		return api.OnResolveResult{}, nil
	}
	p := args.Path
	if !strings.HasPrefix(p, "./") && !strings.HasPrefix(p, "../") && !strings.HasPrefix(p, "/") && p != "." && p != ".." {
		return api.OnResolveResult{}, fmt.Errorf("%q is not a file of the app: scripts import only files of the app, by a path such as \"./label\"", p)
	}
	target := filepath.FromSlash(p)
	if !filepath.IsAbs(target) {
		target = filepath.Join(args.ResolveDir, target)
	}
	rel, ok := a.appRel(target)
	if !ok {
		return api.OnResolveResult{}, nil
	}
	for _, name := range []string{rel, rel + ".ts", strings.TrimSuffix(rel, ".js") + ".ts"} {
		if _, ok := a.generated[name]; ok {
			return api.OnResolveResult{Path: filepath.Join(a.root, filepath.FromSlash(name)), Namespace: "file"}, nil
		}
	}
	return api.OnResolveResult{}, nil
}

// load serves the generated TypeScript files from memory and refuses every other file that
// is not a file of the app.
func (a *assets) load(args api.OnLoadArgs) (api.OnLoadResult, error) {
	if rel, ok := a.appRel(args.Path); ok {
		if data, ok := a.generated[rel]; ok {
			contents := string(data)
			return api.OnLoadResult{Contents: &contents, Loader: api.LoaderTS, ResolveDir: filepath.Dir(args.Path)}, nil
		}
	}
	resolved, err := filepath.EvalSymlinks(args.Path)
	if err != nil {
		return api.OnLoadResult{}, err
	}
	rel, ok := a.local(resolved)
	switch {
	case !ok:
		name := args.Path
		if rel, ok := a.appRel(resolved); ok {
			name = rel
		}
		return api.OnLoadResult{}, fmt.Errorf("%s is outside the app's pages/ and assets/ folders", name)
	case slices.ContainsFunc(strings.Split(rel, "/"), func(e string) bool { return strings.EqualFold(e, "node_modules") }):
		return api.OnLoadResult{}, fmt.Errorf("%s is in a node_modules folder: scripts import only files of the app", rel)
	case path.Base(rel) == "reactive_gen.ts":
		return api.OnLoadResult{}, fmt.Errorf("%s is not generated in this run: only a page with live values or calls has one", rel)
	}
	if _, copied := fileLoaders[path.Ext(rel)]; copied && !embeddable(rel) {
		return api.OnLoadResult{}, fmt.Errorf("%s has a name Go cannot embed: %s", rel, embedRule)
	}
	return api.OnLoadResult{}, nil
}

// appRel returns p relative to the app's folder, when it lies in it.
func (a *assets) appRel(p string) (string, bool) {
	for _, dir := range []string{a.root, a.app.Dir} {
		if rel, err := filepath.Rel(dir, p); err == nil && filepath.IsLocal(rel) {
			return filepath.ToSlash(rel), true
		}
	}
	return "", false
}

// embedRule says which file names go:embed takes.
const embedRule = "use letters, digits, spaces and !#$%&()+,-.=@[]^_{}~, do not end a name with a dot, " +
	"and do not use a name such as CON, NUL or .git"

// embeddable reports whether go:embed takes a file at the slash-separated path p, as the go
// command checks the names of embedded files and folders.
func embeddable(p string) bool {
	if !utf8.ValidString(p) {
		return false
	}
	for elem := range strings.SplitSeq(p, "/") {
		if strings.Count(elem, ".") == len(elem) || strings.HasSuffix(elem, ".") {
			return false
		}
		for _, r := range elem {
			if !embedRune(r) {
				return false
			}
		}
		short, _, _ := strings.Cut(elem, ".")
		if slices.ContainsFunc(windowsNames, func(n string) bool { return strings.EqualFold(n, short) }) || vcsFolders[elem] {
			return false
		}
	}
	return true
}

func embedRune(r rune) bool {
	if r < utf8.RuneSelf {
		return '0' <= r && r <= '9' || 'A' <= r && r <= 'Z' || 'a' <= r && r <= 'z' || strings.ContainsRune("!#$%&()+,-.=@[]^_{}~ ", r)
	}
	return unicode.IsLetter(r)
}

var (
	windowsNames = []string{"CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9"}
	vcsFolders = map[string]bool{".bzr": true, ".hg": true, ".git": true, ".svn": true}
)

func buildErrors(msgs []api.Message) error {
	var d Diagnostics
	for _, m := range msgs {
		pos := ""
		if l := m.Location; l != nil {
			pos = fmt.Sprintf("%s:%d:%d", l.File, l.Line, l.Column+1)
		}
		d = append(d, diag(pos, "E-GEN-036", "%s", m.Text))
	}
	return d.Err()
}

// esbuildWarnings formats esbuild warnings as "<file>:<line>: <text>", or as the text alone when
// esbuild names no file. The file is relative to the app folder, as esbuild names it.
func esbuildWarnings(msgs []api.Message) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.Text
		if l := m.Location; l != nil && l.File != "" {
			out[i] = fmt.Sprintf("%s:%d: %s", l.File, l.Line, m.Text)
		}
	}
	return out
}
