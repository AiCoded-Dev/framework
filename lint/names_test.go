package lint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/docs"
)

func TestGeneratedCodeOnly(t *testing.T) {
	var marked []string
	for _, pkg := range blocks {
		dir := filepath.Join("..", filepath.FromSlash(pkg))
		files, err := os.ReadDir(dir)
		require.NoError(t, err)
		for _, file := range files {
			if !strings.HasSuffix(file.Name(), ".go") || strings.HasSuffix(file.Name(), "_test.go") {
				continue
			}
			f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, file.Name()), nil, parser.ParseComments)
			require.NoError(t, err)
			for _, d := range f.Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok || fn.Doc == nil || !slices.Contains(strings.Split(fn.Doc.Text(), "\n"), "Generated code only.") {
					continue
				}
				name := fn.Name.Name
				if fn.Recv != nil {
					name = receiver(fn.Recv.List[0].Type) + "." + name
				}
				marked = append(marked, pkg+"."+name)
			}
		}
	}
	slices.Sort(marked)
	assert.Equal(t, forGeneratedCode, marked, "sorted, and each with the doc line Generated code only.")
}

func TestNamesPages(t *testing.T) {
	page := func(code string) string {
		data, err := fs.ReadFile(docs.FS, "errors/"+code+".md")
		require.NoError(t, err)
		return string(data)
	}
	for _, name := range forGeneratedCode {
		assert.Contains(t, page("E-LINT-005"), "`"+display(Framework+"/"+name)+"`", "the page names every function for generated code")
	}
	for _, names := range restricted {
		for _, name := range names {
			name = strings.Replace(name, "*", "...", 1)
			assert.Contains(t, page("E-LINT-009"), "`"+name+"`", "the page names every name apps may use")
		}
	}
}

// receiver returns the name of the type of a method's receiver.
func receiver(x ast.Expr) string {
	for {
		switch e := x.(type) {
		case *ast.StarExpr:
			x = e.X
		case *ast.IndexExpr:
			x = e.X
		case *ast.IndexListExpr:
			x = e.X
		case *ast.Ident:
			return e.Name
		default:
			return ""
		}
	}
}
