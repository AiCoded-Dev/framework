package generate

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
)

// listTree returns every file under dir with its content, and every folder with a trailing /.
func listTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil || d.IsDir() {
			out[filepath.ToSlash(rel)+"/"] = ""
			return err
		}
		data, err := os.ReadFile(p)
		out[filepath.ToSlash(rel)] = string(data)
		return err
	}))
	return out
}

func TestRunFrozenCleanApp(t *testing.T) {
	dir := copyApp(t, "app")
	require.NoError(t, Generate(dir))
	res, err := Run(dir, Options{Frozen: true})
	require.NoError(t, err)
	assert.Empty(t, res.Changed)
}

func TestRunFrozenReportsDrift(t *testing.T) {
	dir := copyApp(t, "app")
	require.NoError(t, Generate(dir))
	write(t, dir, "pages/notes/route_gen.go", "package notes\n")
	require.NoError(t, os.Remove(filepath.Join(dir, "pages", "live", "reactive_gen.ts")))
	write(t, dir, "pages/gone/route_gen.go", "package gone\n")
	before := listTree(t, dir)

	res, err := Run(dir, Options{Frozen: true})
	var d Diagnostics
	require.ErrorAs(t, err, &d)
	var got []string
	for _, e := range d {
		assert.Equal(t, "E-CHK-001", e.Code)
		assert.Equal(t, "run aicoded generate in "+dir+" and keep the result; never edit generated files by hand", e.Fix)
		got = append(got, e.Pos+" "+e.Msg)
	}
	assert.Equal(t, []string{
		"pages/gone/route_gen.go:1 pages/gone/route_gen.go should not exist",
		"pages/live/reactive_gen.ts:1 pages/live/reactive_gen.ts is missing",
		"pages/notes/route_gen.go:1 pages/notes/route_gen.go is out of date",
	}, got)
	assert.Equal(t, []string{"pages/gone/route_gen.go", "pages/live/reactive_gen.ts", "pages/notes/route_gen.go"}, res.Changed)
	assert.Equal(t, before, listTree(t, dir), "a frozen run writes nothing")
}

func TestRunChanged(t *testing.T) {
	dir := rpcApp(t, billingRPC)
	res, err := Run(dir, Options{})
	require.NoError(t, err)
	assert.Equal(t, []string{".aicoded/rpc.json", "aicoded.yaml", "rpc/server_gen.go"}, res.Changed)

	res, err = Run(dir, Options{})
	require.NoError(t, err)
	assert.Empty(t, res.Changed, "a file written with the content it has is not a change")

	require.NoError(t, os.RemoveAll(filepath.Join(dir, "rpc")))
	res, err = Run(dir, Options{})
	require.NoError(t, err)
	assert.Equal(t, []string{".aicoded/rpc.json", "aicoded.yaml"}, res.Changed, "a removed file is a change")
}

func TestRunFiles(t *testing.T) {
	dir := copyApp(t, "app")
	res, err := Run(dir, Options{})
	require.NoError(t, err)
	var named []string
	for rel := range listTree(t, dir) {
		if strings.HasSuffix(rel, "_gen.go") || strings.HasSuffix(rel, "_gen.ts") ||
			strings.HasPrefix(rel, "pages/assets_gen/") && !strings.HasSuffix(rel, "/") {
			named = append(named, rel)
		}
	}
	slices.Sort(named)
	assert.Equal(t, named, res.Files, "the generated pages and assets, and no data provider")

	billing, shop := pair(t)
	res, err = Run(billing, Options{Frozen: true})
	require.NoError(t, err)
	assert.Equal(t, []string{".aicoded/rpc.json", "rpc/server_gen.go"}, res.Files)
	res, err = Run(shop, Options{Frozen: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"services/billing/client_gen.go"}, res.Files)
}

// pingRPC adds a second function to billingRPC.
const pingRPC = `
//ssr:access caller=shop apps=true
func Ping(ctx context.Context, in GetIn) (GetIn, error) { return in, nil }
`

// pair writes billing, which serves Get and Ping, and shop, which holds its snapshot as aicoded
// rpc add leaves it, in two folders next to each other.
func pair(t *testing.T) (billing, shop string) {
	t.Helper()
	root := t.TempDir()
	billing, shop = filepath.Join(root, "billing"), filepath.Join(root, "shop")
	write(t, billing, "go.mod", "module example.com/billing\n\ngo 1.25.0\n")
	write(t, billing, "aicoded.yaml", "app: billing\n")
	write(t, billing, "rpc/billing.go", billingRPC+pingRPC)
	require.NoError(t, Generate(billing))
	write(t, shop, "go.mod", "module example.com/shop\n\ngo 1.25.0\n")
	write(t, shop, "aicoded.yaml", "app: shop\n")
	write(t, shop, ".aicoded/services/billing.json", read(t, billing, ".aicoded/rpc.json"))
	require.NoError(t, Generate(shop))
	return billing, shop
}

// requireBreakRefused removes Ping from billing and checks that Run with opts refuses the change
// at shop's held snapshot and writes nothing into billing.
func requireBreakRefused(t *testing.T, billing, shop string, opts Options) {
	t.Helper()
	before := listTree(t, billing)
	write(t, billing, "rpc/billing.go", billingRPC)
	_, err := Run(billing, opts)
	var e *errs.Error
	require.ErrorAs(t, err, &e)
	assert.Equal(t, "E-RPC-013", e.Code)
	assert.Equal(t, filepath.Join(shop, ".aicoded", "services", "billing.json")+":8", e.Pos, "the line of Ping in the held snapshot")
	assert.Equal(t, "shop holds a snapshot of billing that billing no longer matches: Ping: function removed", e.Msg)
	assert.Equal(t, "undo the change to the rpc/ folder of billing, or run aicoded rpc add billing in shop and update its calls", e.Fix)
	after := listTree(t, billing)
	for _, rel := range []string{".aicoded/rpc.json", "rpc/server_gen.go", "aicoded.yaml"} {
		assert.Equal(t, before[rel], after[rel], rel)
	}
}

func TestRunRefusesBreakBeforeWrite(t *testing.T) {
	billing, shop := pair(t)
	requireBreakRefused(t, billing, shop, Options{Workspace: map[string]string{"billing": billing, "shop": shop}})
}

func TestRunSiblingsAreTheWorkspace(t *testing.T) {
	billing, shop := pair(t)
	requireBreakRefused(t, billing, shop, Options{})

	_, err := Run(billing, Options{Workspace: map[string]string{"billing": billing}})
	require.NoError(t, err, "a workspace without shop does not check its snapshot")
	assert.NotContains(t, read(t, billing, ".aicoded/rpc.json"), "Ping")
}

func TestRunRefusesTwoHoldersWithOneName(t *testing.T) {
	billing, shop := pair(t)
	other := filepath.Join(filepath.Dir(shop), "shop-copy")
	require.NoError(t, os.CopyFS(other, os.DirFS(shop)))

	_, err := Run(billing, Options{})
	var e *errs.Error
	require.ErrorAs(t, err, &e)
	assert.Equal(t, "E-DEV-010", e.Code)
	assert.Equal(t, "the apps in "+shop+" and "+other+" are both named shop", e.Msg)
}

func TestRunCompatibleChangeWrites(t *testing.T) {
	billing, _ := pair(t)
	before := snapshot(t, billing)
	src := billingRPC + pingRPC + `
//ssr:access caller=shop apps=true
func Pong(ctx context.Context, in GetOut) (GetOut, error) { return in, nil }
`
	write(t, billing, "rpc/billing.go", src)
	_, err := Run(billing, Options{})
	require.NoError(t, err)
	after := snapshot(t, billing)
	assert.Contains(t, after.Methods, "Pong")
	assert.Equal(t, before.Types, after.Types, "no field is renumbered")
}

func TestRunWarnings(t *testing.T) {
	app := newApp(t, map[string]string{
		"pages/index.html": doc(`<head><ssr:access role="*"/><ssr:assets/></head><body></body>`),
		"pages/index.ts":   "console.log({a: 1, a: 2});\n",
	})
	res, err := Run(app.Dir, Options{})
	require.NoError(t, err)
	assert.Equal(t, []string{`pages/index.ts:1: Duplicate key "a" in object literal`}, res.Warnings)
}

func TestServicesFolderGuard(t *testing.T) {
	dir := shopApp(t)
	write(t, dir, "services/billing/extra.go", "package billing\n")
	before := listTree(t, dir)
	_, err := Run(dir, Options{})
	var e *errs.Error
	require.ErrorAs(t, err, &e)
	assert.Equal(t, "E-RPC-014", e.Code)
	assert.Equal(t, "services/billing/extra.go", e.Pos)
	assert.Equal(t, "move services/billing/extra.go out of services/billing/; that folder holds only the generated client", e.Fix)
	assert.Equal(t, before, listTree(t, dir), "nothing is written")
}

// TestRunModule generates an app that lies inside an enclosing module and has no go.mod of its
// own: its pages import it by the path Module names, and the enclosing module builds them.
func TestRunModule(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	ui := filepath.Join(copyApp(t, "app"), "internal", "ui")
	write(t, ui, "deps/deps.go", "// Package deps is what the pages share.\npackage deps\n\n// Deps is passed to every page.\ntype Deps struct{}\n")
	write(t, ui, "pages/index.html", doc(`<head><ssr:access role="*"/></head><body><ssr:content default="/a"/></body>`))
	write(t, ui, "pages/a/index.html", `<p>a</p>`)

	_, err := Run(ui, Options{})
	assert.Equal(t, "E-GEN-037", errs.Code(err), "without Module an app needs its own go.mod")

	_, err = Run(ui, Options{Module: "example.com/app/internal/ui"})
	require.NoError(t, err)
	handler := read(t, ui, "pages/handler_gen.go")
	assert.Contains(t, handler, `"example.com/app/internal/ui/deps"`)
	assert.Contains(t, handler, `routeA "example.com/app/internal/ui/pages/a"`)
	goVet(t, ui)
}
