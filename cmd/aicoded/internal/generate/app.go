package generate

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/template"
	"aicoded.dev/framework/internal/errs"
)

// App is an app's source tree: pages/, an optional deps/ package and go.mod.
type App struct {
	Dir    string
	Module string
	Deps   bool
}

// loadApp reads the app in dir. module is its import path; with "", the go.mod in dir names it.
func loadApp(dir, module string) (App, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return App{}, err
	}
	if module == "" {
		if module, err = goModule(abs); err != nil {
			return App{}, err
		}
	}
	st, err := os.Stat(filepath.Join(abs, "deps"))
	return App{Dir: abs, Module: module, Deps: err == nil && st.IsDir()}, nil
}

// goModule returns the module path of the go.mod in dir, an absolute path.
func goModule(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if errors.Is(err, fs.ErrNotExist) {
		return "", diag("", "E-GEN-037", "%s has no go.mod", dir)
	}
	if err != nil {
		return "", err
	}
	module := modulePath(data)
	if module == "" {
		return "", diag(filepath.Join(dir, "go.mod")+":1", "E-GEN-037", "go.mod has no module line")
	}
	return module, nil
}

func modulePath(gomod []byte) string {
	s := bufio.NewScanner(bytes.NewReader(gomod))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if rest, ok := strings.CutPrefix(line, "module"); ok && (rest == "" || rest[0] == ' ' || rest[0] == '\t') {
			rest, _, _ = strings.Cut(rest, "//")
			rest = strings.TrimSpace(rest)
			if p, err := strconv.Unquote(rest); err == nil {
				return p
			}
			return rest
		}
	}
	return ""
}

// diag returns a coded generator error at pos with the code's fix line. It panics when the
// code has no fix line, because that is a bug in the generator.
func diag(pos, code, format string, args ...any) *errs.Error {
	fix := template.Fix(code)
	if fix == "" {
		panic("generate: no fix line for " + code)
	}
	return errs.At(pos, code, fmt.Sprintf(format, args...), fix)
}
