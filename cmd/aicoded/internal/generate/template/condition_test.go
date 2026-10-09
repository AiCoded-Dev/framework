package template

import (
	"go/ast"
	goparser "go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A chain of ssr:if, ssr:else-if and ssr:else is written as if statements nested in else blocks,
// never as an else-if chain, which staticcheck reports as QF1003 when it compares one value; each
// condition keeps its template line.
func TestConditionChainIsNested(t *testing.T) {
	code := pageCode(t, "<ssr:var name=\"status\" type=\"string\"/>\n<p ssr:if=\"status == 'a'\">a</p>\n"+
		"<p ssr:else-if=\"status == 'b'\">b</p>\n<p ssr:else-if=\"status == 'c'\">c</p>\n<p ssr:else>d</p>\n")
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, "route_gen.go", "package p\n\nfunc f() error {\n"+code+"return nil\n}\n", goparser.ParseComments)
	require.NoError(t, err, code)
	lines := map[string]int{}
	ast.Inspect(f, func(n ast.Node) bool {
		if s, ok := n.(*ast.IfStmt); ok {
			_, chain := s.Else.(*ast.IfStmt)
			assert.False(t, chain, "an else-if chain at %s", fset.Position(s.Pos()))
			if b, ok := s.Cond.(*ast.BinaryExpr); ok && b.Op == token.EQL {
				lines[b.Y.(*ast.BasicLit).Value] = fset.Position(b.Pos()).Line
			}
		}
		return true
	})
	assert.Equal(t, map[string]int{`"a"`: 2, `"b"`: 3, `"c"`: 4}, lines)
}
