package lint

import (
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

func TestDescribe(t *testing.T) {
	assert.Equal(t, "a form value", describe(formValue|input(0)))
	assert.Equal(t, "a form value or the request", describe(formValue|request))
	assert.Equal(t, "a URL parameter, the viewer's identity or the answer of another app", describe(appAnswer|viewer|urlParam))
}

// BenchmarkTaint measures the analysis of E-LINT-008 alone on examples/people.
func BenchmarkTaint(b *testing.B) {
	if _, err := exec.LookPath("go"); err != nil {
		b.Skip("go toolchain not in PATH")
	}
	dir, err := filepath.Abs(filepath.Join("..", "examples", "people"))
	require.NoError(b, err)
	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.LoadAllSyntax | packages.NeedModule, Context: b.Context(), Dir: dir,
		Env: append(os.Environ(), "CGO_ENABLED=0"),
	}, "./...")
	require.NoError(b, err)
	require.NoError(b, loadError(pkgs))
	for b.Loop() {
		require.NoError(b, (&app{dir: dir}).taint(b.Context(), pkgs))
	}
}

func TestTaintFailsClosed(t *testing.T) {
	require.ErrorContains(t, (&app{}).taint(t.Context(), nil), "no package belongs to the app's module")
	illTyped := &packages.Package{
		PkgPath: "example.com/app", Module: &packages.Module{Path: "example.com/app", Main: true}, IllTyped: true,
	}
	require.ErrorContains(t, (&app{}).taint(t.Context(), []*packages.Package{illTyped}), "package example.com/app cannot be built")
}

func TestWritersHoldEverySetter(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	pkgs, err := packages.Load(&packages.Config{Mode: packages.NeedName | packages.NeedTypes, Context: t.Context(), Dir: ".."},
		prefixed(Framework+"/", blocks)...)
	require.NoError(t, err)
	require.NoError(t, loadError(pkgs))
	var setters, missing []string
	for _, p := range pkgs {
		for _, name := range p.Types.Scope().Names() {
			tn, ok := p.Types.Scope().Lookup(name).(*types.TypeName)
			if !ok || tn.IsAlias() {
				continue
			}
			n := tn.Type().(*types.Named)
			methods := n.Methods()
			if i, ok := n.Underlying().(*types.Interface); ok {
				methods = i.ExplicitMethods()
			}
			for m := range methods {
				form := p.PkgPath == Framework+"/web/form"
				if m.Exported() && (strings.HasPrefix(m.Name(), "Set") || form && strings.HasPrefix(m.Name(), "Write")) {
					setters = append(setters, member(m))
				}
			}
		}
	}
	assert.Contains(t, setters, Framework+"/web/form.Input.SetError")
	assert.Contains(t, setters, Framework+"/web/form.SelectOptionElement.WriteHtml")
	for _, s := range setters {
		if writers[s] == nil {
			missing = append(missing, s)
		}
	}
	assert.Empty(t, missing, "a setter of a building block writes its arguments into its receiver, and a Write method of a field into its writer")
}
