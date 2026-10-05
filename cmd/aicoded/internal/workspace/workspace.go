// Package workspace finds the apps of a workspace: the folders under its root that hold a
// permission list.
package workspace

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"aicoded.dev/framework/cmd/aicoded/internal/manifest"
	"aicoded.dev/framework/internal/errs"
)

// skipped are the folders Discover does not look into, besides those whose names start with .
// or _.
var skipped = map[string]bool{"vendor": true, "node_modules": true, "testdata": true}

// Discover returns the absolute paths of the folders under root, root included, that hold
// aicoded.yaml, sorted. It resolves the symbolic links in root's path, and follows none below
// it. It does not look inside an app's folder, nor inside folders whose names start with . or _,
// or are vendor, node_modules or testdata.
func Discover(root string) ([]string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if root, err = filepath.EvalSymlinks(root); err != nil {
		return nil, err
	}
	var apps []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case !d.IsDir():
			return nil
		case p != root && (strings.HasPrefix(d.Name(), ".") || strings.HasPrefix(d.Name(), "_") || skipped[d.Name()]):
			return filepath.SkipDir
		}
		if _, err := os.Lstat(filepath.Join(p, manifest.FileName)); err == nil {
			apps = append(apps, p)
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(apps)
	return apps, nil
}

// Find returns the folder of the app named app among the apps Discover finds under root. It
// refuses, with E-RPC-007, a name that no app there has, or that two have.
func Find(root, app string) (string, error) {
	dirs, err := Discover(root)
	if err != nil {
		return "", err
	}
	var found []string
	for _, dir := range dirs {
		m, err := manifest.Load(filepath.Join(dir, manifest.FileName))
		if err != nil {
			return "", err
		}
		if m.App == app {
			found = append(found, dir)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return "", errs.New("E-RPC-007", fmt.Sprintf("no app under %s is named %s", root, app),
			"name an app from the app: line of its aicoded.yaml, and keep its folder next to this app's folder")
	}
	return "", errs.New("E-RPC-007", fmt.Sprintf("more than one app under %s is named %s: %s", root, app, strings.Join(found, ", ")),
		"give every app under the folder its own name on the app: line of its aicoded.yaml")
}
