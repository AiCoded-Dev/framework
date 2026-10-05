// Package scaffold creates new apps.
package scaffold

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"aicoded.dev/framework/cmd/aicoded/internal/generate"
	"aicoded.dev/framework/cmd/aicoded/internal/gotool"
	"aicoded.dev/framework/cmd/aicoded/internal/manifest"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/lint"
)

const fixName = "choose a name of lowercase letters, digits and dashes that no folder here has"

// agents is AGENTS.md, the rules for AI assistants that every new app gets.
//
//go:embed AGENTS.md
var agents string

// Create makes the app name in a new folder under parent and returns the folder. It writes
// nothing when it fails.
func Create(ctx context.Context, parent, name string, fw Framework, out io.Writer) (string, error) {
	if !manifest.ValidApp(name) {
		return "", errs.New("E-CLI-002", fmt.Sprintf("%q is not a valid app name", name), fixName)
	}
	if fw.Version == "" && fw.Dir == "" {
		return "", noFramework()
	}
	parent, err := filepath.Abs(parent)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(parent, name)
	if _, err := os.Lstat(dir); err == nil {
		return "", errs.New("E-CLI-002", dir+" already exists", fixName)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	tmp, err := os.MkdirTemp(parent, "."+name+"-")
	if err != nil {
		return "", err
	}
	if err := fill(ctx, tmp, name, fw); err != nil {
		_ = os.RemoveAll(tmp)
		return "", err
	}
	if err := os.Rename(tmp, dir); err != nil {
		_ = os.RemoveAll(tmp)
		return "", err
	}
	fmt.Fprintf(out, "aicoded init: created %s\n", dir)
	if fw.Dir != "" {
		fmt.Fprintf(out, "aicoded init: %s/go.mod uses the framework checkout at %s; the delivery pipeline will refuse a replace line, so require a released version before you publish\n", name, fw.Dir)
	}
	return dir, nil
}

// fill writes the files of the app name into dir, generates it and tidies its go.mod.
func fill(ctx context.Context, dir, name string, fw Framework) error {
	files := map[string]string{
		"AGENTS.md":        agents,
		"go.mod":           goMod(name, fw),
		manifest.FileName:  "app: " + name + "\n",
		"main.go":          fmt.Sprintf(mainGo, name),
		"pages/index.html": fmt.Sprintf(indexHTML, name, name),
	}
	for _, rel := range slices.Sorted(maps.Keys(files)) {
		if err := generate.WriteFile(dir, rel, []byte(files[rel])); err != nil {
			return err
		}
	}
	if _, err := generate.Run(dir, generate.Options{Workspace: map[string]string{}}); err != nil {
		return err
	}
	err := gotool.Tidy(ctx, dir)
	var e *errs.Error
	if errors.As(err, &e) && e.Code == "E-CHK-006" {
		tidy := *e
		tidy.Fix = "check the network, then run aicoded init " + name + " again"
		return &tidy
	}
	return err
}

// goMod returns the go.mod of the app name, which requires the framework fw.
func goMod(name string, fw Framework) string {
	if fw.Dir == "" {
		return fmt.Sprintf("module %s\n\ngo 1.25.0\n\nrequire %s %s\n", name, lint.Framework, fw.Version)
	}
	return fmt.Sprintf("module %s\n\ngo 1.25.0\n\nrequire %s %s\n\nreplace %s => %s\n",
		name, lint.Framework, unreleased, lint.Framework, strconv.Quote(fw.Dir))
}

const mainGo = `package main

import (
	"aicoded.dev/framework/app"

	"%s/pages"
)

func main() {
	app.Main(app.Options{Handler: pages.NewHandler()})
}
`

const indexHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<ssr:access role="*"/>
<title>%s</title>
<ssr:assets/>
</head>
<body>
<h1>%s</h1>
</body>
</html>
`
