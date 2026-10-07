package generate

import (
	"bytes"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/manifest"
)

// Options says how Run generates an app.
type Options struct {
	// Workspace maps the name of every app of the workspace to its folder. With nil, the
	// workspace is the apps in the folders directly next to dir, and two of them with one name
	// that hold a snapshot of this app are E-DEV-010.
	Workspace map[string]string
	// Frozen writes nothing; every generated file that would change is an E-CHK-001 problem.
	Frozen bool
	// Module is the import path of dir, for an app that lies inside an enclosing module and has
	// no go.mod of its own. With "", the go.mod in dir names it.
	Module string
}

// Result is what Run changed and what it warns about.
type Result struct {
	// Changed lists the generated files written, or with Frozen the ones that differ,
	// relative to the app folder, slash-separated and sorted. Removed files are included.
	Changed []string
	// Files lists every file the generator owns in the app: the generated Go and TypeScript
	// files, the built assets and the snapshot of the functions the app serves, relative to the
	// app folder, slash-separated and sorted. Every other file of the app is hand-written.
	Files []string
	// Warnings are the esbuild warnings, each "<file>:<line>: <text>".
	Warnings []string
}

// Run generates the app in dir as Generate describes. Before it writes anything, it checks the
// new schema of the app's functions against every snapshot of them held by another app of the
// workspace, and refuses a change that breaks one with E-RPC-013.
func Run(dir string, opts Options) (Result, error) {
	app, err := loadApp(dir, opts.Module)
	if err != nil {
		return Result{}, err
	}
	g := newGen(app)
	res, err := g.run(opts)
	res.Warnings = g.assets.warnings
	return res, err
}

func (g *gen) run(opts Options) (Result, error) {
	var routes []*Route
	if _, err := os.Stat(filepath.Join(g.app.Dir, "pages")); !errors.Is(err, fs.ErrNotExist) {
		if routes, err = discover(g.app, g.assets.image); err != nil {
			return Result{}, err
		}
		if err := g.build(routes); err != nil {
			return Result{}, err
		}
	}
	serves, err := g.callee()
	if err != nil {
		return Result{}, err
	}
	calls, err := g.callers()
	if err != nil {
		return Result{}, err
	}
	if err := g.sections(routes, manifest.Services{Calls: calls, Serves: serves}); err != nil {
		return Result{}, err
	}
	if err := g.compatible(opts.Workspace); err != nil {
		return Result{}, err
	}
	changes, err := g.changes()
	if err != nil {
		return Result{}, err
	}
	res := Result{Changed: slices.Sorted(maps.Keys(changes)), Files: g.owned()}
	if opts.Frozen {
		var d Diagnostics
		for _, rel := range res.Changed {
			d = append(d, errs.At(rel+":1", "E-CHK-001", rel+" "+changes[rel],
				"run aicoded generate in "+g.app.Dir+" and keep the result; never edit generated files by hand"))
		}
		return res, d.Err()
	}
	if err := g.write(); err != nil {
		return Result{}, err
	}
	return res, nil
}

// owned returns the files the generator owns in the app, as Result.Files lists them.
func (g *gen) owned() []string {
	files := slices.Collect(maps.Keys(g.files))
	for rel := range g.assets.files {
		files = append(files, path.Join("pages", assetsDir, rel))
	}
	if g.schema != nil {
		files = append(files, snapshotFile)
	}
	slices.Sort(files)
	return files
}

// changes returns what write would change, by path relative to the app: a file it writes that
// holds other content "is out of date", one that is not there "is missing", and one it removes
// "should not exist". It only reads the app's folder.
func (g *gen) changes() (map[string]string, error) {
	root, err := os.OpenRoot(g.app.Dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	out := map[string]string{}
	want := maps.Clone(g.files)
	maps.Copy(want, g.snapshots)
	if g.manifest != nil {
		want[manifestFile] = g.manifest
	}
	var gone []string
	assetsPath := path.Join("pages", assetsDir)
	st, err := root.Lstat(filepath.FromSlash(assetsPath))
	switch {
	case err == nil && st.IsDir():
		err = fs.WalkDir(root.FS(), assetsPath, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if _, ok := g.assets.files[strings.TrimPrefix(p, assetsPath+"/")]; !ok {
				gone = append(gone, p)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		for rel, data := range g.assets.files {
			want[path.Join(assetsPath, rel)] = data
		}
	case err == nil:
		gone = append(gone, assetsPath)
		for rel := range g.assets.files {
			out[path.Join(assetsPath, rel)] = "is missing"
		}
	case errors.Is(err, fs.ErrNotExist):
		for rel := range g.assets.files {
			out[path.Join(assetsPath, rel)] = "is missing"
		}
	default:
		return nil, err
	}
	if _, err := root.Stat("pages"); err == nil {
		err = fs.WalkDir(root.FS(), "pages", func(p string, d fs.DirEntry, err error) error {
			switch {
			case err != nil:
				return err
			case d.IsDir() && p == assetsPath:
				return fs.SkipDir
			case d.IsDir() || !generatedNames[d.Name()]:
				return nil
			}
			if _, ok := g.files[p]; !ok {
				gone = append(gone, p)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	for rel, data := range want {
		state, err := compare(root, rel, data)
		if err != nil {
			return nil, err
		}
		if state != "" {
			out[rel] = state
		}
	}
	for rel := range g.stubs {
		if _, err := root.Lstat(filepath.FromSlash(rel)); errors.Is(err, fs.ErrNotExist) {
			out[rel] = "is missing"
		} else if err != nil {
			return nil, err
		}
	}
	for _, rel := range append(gone, g.stale...) {
		if _, err := root.Lstat(filepath.FromSlash(rel)); err == nil {
			out[rel] = "should not exist"
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	return out, nil
}

// compare returns how the file at rel in root differs from data: "" when it holds data, "is
// missing" when nothing is there, and "is out of date" otherwise.
func compare(root *os.Root, rel string, data []byte) (string, error) {
	name := filepath.FromSlash(rel)
	st, err := root.Lstat(name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "is missing", nil
	case err != nil:
		return "", err
	case !st.Mode().IsRegular():
		return "is out of date", nil
	}
	have, err := root.ReadFile(name)
	if err != nil {
		return "", err
	}
	if bytes.Equal(have, data) {
		return "", nil
	}
	return "is out of date", nil
}
