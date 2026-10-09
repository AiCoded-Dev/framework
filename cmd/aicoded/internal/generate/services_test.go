package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/rpcschema"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/manifest"
)

// billingRPC is the rpc/ package of an app that serves one function.
const billingRPC = `package rpc

import "context"

type GetIn struct {
	ID int64
}

type GetOut struct {
	Total int64
}

//ssr:access caller=shop role=editor|admin apps=true
func Get(ctx context.Context, in GetIn) (GetOut, error) { return GetOut{}, nil }
`

// rpcApp writes an app named billing with no pages whose rpc/billing.go is src.
func rpcApp(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/billing\n\ngo 1.26.0\n")
	write(t, dir, "aicoded.yaml", "# billing app\napp: billing\n")
	write(t, dir, "rpc/billing.go", src)
	return dir
}

func read(t *testing.T, dir, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	require.NoError(t, err)
	return string(data)
}

func snapshot(t *testing.T, dir string) rpcschema.Schema {
	t.Helper()
	s, err := rpcschema.Load(filepath.Join(dir, ".aicoded", "rpc.json"))
	require.NoError(t, err)
	return s
}

func TestServicesSection(t *testing.T) {
	section, err := servicesSection(manifest.Services{
		Calls: map[string][]string{"billing": {"GetInvoice", "Ping"}},
		Serves: map[string]manifest.Serve{
			"Order": {Callers: []string{"reports", "shop"}, Require: []string{"admin|editor"}},
			"Ping":  {Callers: []string{"shop"}, Apps: true},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, servicesMarker+`
services:
  calls:
    "billing": ["GetInvoice", "Ping"]
  serves:
    "Order":
      callers: ["reports", "shop"]
      require: ["admin|editor"]
    "Ping":
      callers: ["shop"]
      apps: true
`, string(section), "a function without role= has no require rules, so no viewer may call it")

	empty, err := servicesSection(manifest.Services{})
	require.NoError(t, err)
	assert.Equal(t, servicesMarker+"\nservices: {}\n", string(empty))
}

func TestWithServicesRefuses(t *testing.T) {
	section, err := servicesSection(manifest.Services{Calls: map[string][]string{"billing": {"Get"}}})
	require.NoError(t, err)
	for name, c := range map[string]struct{ data, pos string }{
		"services twice":                   {"services: {}\napp: x\nservices: {}\n", "aicoded.yaml:3"},
		"services continued at column one": {"services: [a,\nb]\napp: x\n", "aicoded.yaml:1"},
		"flow style":                       {"{app: x, services: {}}\n", "aicoded.yaml:1"},
		"indented keys":                    {"  app: x\n  services: {}\n  secrets: [a]\n", "aicoded.yaml:2"},
	} {
		_, err := withSection([]byte(c.data), "services", servicesMarker, section)
		var e *errs.Error
		if assert.ErrorAs(t, err, &e, name) {
			assert.Equal(t, "E-MAN-003", e.Code, name)
			assert.Equal(t, c.pos, e.Pos, name)
		}
	}
}

func TestGenerateCallee(t *testing.T) {
	dir := rpcApp(t, billingRPC)
	require.NoError(t, Generate(dir))

	assert.Equal(t, "# billing app\napp: billing\n"+accessMarker+"\naccess: {}\n"+servicesMarker+`
services:
  serves:
    "Get":
      callers: ["shop"]
      require: ["admin|editor"]
      apps: true
`, read(t, dir, "aicoded.yaml"))
	assert.Contains(t, read(t, dir, "rpc/server_gen.go"), "func Server() *frameworkrpc.Server {")
	assert.Equal(t, rpcschema.Schema{
		App:     "billing",
		Methods: map[string]rpcschema.Method{"Get": {In: "GetIn", Out: "GetOut"}},
		Types: map[string]rpcschema.Type{
			"GetIn":  {Fields: map[string]rpcschema.Field{"ID": {Number: 1, Type: "int64"}}},
			"GetOut": {Fields: map[string]rpcschema.Field{"Total": {Number: 1, Type: "int64"}}},
		},
	}, snapshot(t, dir))
	st, err := os.Stat(filepath.Join(dir, ".aicoded"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), st.Mode().Perm())

	before := map[string]string{}
	for _, rel := range []string{"aicoded.yaml", "rpc/server_gen.go", ".aicoded/rpc.json"} {
		before[rel] = read(t, dir, rel)
	}
	require.NoError(t, Generate(dir))
	for rel, data := range before {
		assert.Equal(t, data, read(t, dir, rel), "a second run changes nothing in %s", rel)
	}
}

func TestGenerateRetiresRenamedFields(t *testing.T) {
	dir := rpcApp(t, billingRPC)
	require.NoError(t, Generate(dir))
	write(t, dir, "rpc/billing.go", strings.Replace(billingRPC, "\tID int64", "\tKey int64", 1))
	require.NoError(t, Generate(dir))
	assert.Equal(t, rpcschema.Type{Fields: map[string]rpcschema.Field{"Key": {Number: 2, Type: "int64"}}, Retired: []int{1}},
		snapshot(t, dir).Types["GetIn"], "a renamed field is a new field; the old number is never used again")
}

func TestGenerateNumbersAFieldAddedBackAfresh(t *testing.T) {
	dir := rpcApp(t, billingRPC)
	require.NoError(t, Generate(dir))
	held := read(t, dir, ".aicoded/rpc.json")
	write(t, dir, "rpc/billing.go", strings.Replace(billingRPC, "\tID int64\n", "", 1))
	require.NoError(t, Generate(dir))
	write(t, dir, "rpc/billing.go", billingRPC)
	require.NoError(t, Generate(dir))
	assert.Equal(t, rpcschema.Type{Fields: map[string]rpcschema.Field{"ID": {Number: 2, Type: "int64"}}, Retired: []int{1}},
		snapshot(t, dir).Types["GetIn"], "undoing the change in rpc/ alone does not give the field its number back")

	write(t, dir, ".aicoded/rpc.json", held)
	require.NoError(t, Generate(dir))
	assert.Equal(t, held, read(t, dir, ".aicoded/rpc.json"), "restoring the snapshot too does")
}

func TestGenerateWithoutFunctions(t *testing.T) {
	dir := rpcApp(t, billingRPC)
	require.NoError(t, Generate(dir))
	write(t, dir, "rpc/billing.go", "package rpc\n")
	require.NoError(t, Generate(dir))
	assert.NoFileExists(t, filepath.Join(dir, "rpc", "server_gen.go"))
	assert.Equal(t, "# billing app\napp: billing\n"+accessMarker+"\naccess: {}\n"+servicesMarker+"\nservices: {}\n",
		read(t, dir, "aicoded.yaml"), "the section stays, empty, once it was written")
	assert.Empty(t, snapshot(t, dir).Methods)

	require.NoError(t, os.RemoveAll(filepath.Join(dir, "rpc")))
	require.NoError(t, Generate(dir))
	assert.NoFileExists(t, filepath.Join(dir, ".aicoded", "rpc.json"), "an app without rpc/ has no snapshot")
}

func TestGenerateRefusesBrokenSnapshot(t *testing.T) {
	dir := rpcApp(t, billingRPC)
	write(t, dir, ".aicoded/rpc.json", "{")
	assert.Equal(t, "E-RPC-005", errs.Code(Generate(dir)))
	assert.NoFileExists(t, filepath.Join(dir, "rpc", "server_gen.go"), "nothing is written")
	assert.Equal(t, "# billing app\napp: billing\n", read(t, dir, "aicoded.yaml"))
	assert.Equal(t, "{", read(t, dir, ".aicoded/rpc.json"))
}

func TestGenerateSplicesServices(t *testing.T) {
	dir := rpcApp(t, billingRPC)
	write(t, dir, "aicoded.yaml", "# billing app\napp: billing # the name\nservices:\n  serves: {}\n\n# secrets below\nsecrets: [api_key]\n")
	require.NoError(t, Generate(dir))
	section, err := servicesSection(manifest.Services{Serves: map[string]manifest.Serve{
		"Get": {Callers: []string{"shop"}, Require: []string{"admin|editor"}, Apps: true},
	}})
	require.NoError(t, err)
	assert.Equal(t, "# billing app\napp: billing # the name\n"+string(section)+"\n# secrets below\nsecrets: [api_key]\n"+
		accessMarker+"\naccess: {}\n", read(t, dir, "aicoded.yaml"))
}

func TestGenerateLeavesServicesOut(t *testing.T) {
	dir := copyApp(t, "app")
	require.NoError(t, Generate(dir))
	assert.NotContains(t, read(t, dir, "aicoded.yaml"), "services", "an app that neither calls nor serves has no services section")
	assert.NoDirExists(t, filepath.Join(dir, ".aicoded"))
}

func TestGenerateCalleeNeedsManifest(t *testing.T) {
	dir := rpcApp(t, billingRPC)
	require.NoError(t, os.Remove(filepath.Join(dir, "aicoded.yaml")))
	assert.Equal(t, "E-MAN-006", errs.Code(Generate(dir)))
}

// heldBilling is the snapshot of billing that a calling app holds.
var heldBilling = rpcschema.Schema{
	App:     "billing",
	Methods: map[string]rpcschema.Method{"Get": {In: "GetIn", Out: "GetOut"}, "Ping": {In: "GetIn", Out: "GetIn"}},
	Types: map[string]rpcschema.Type{
		"GetIn":  {Fields: map[string]rpcschema.Field{"ID": {Number: 1, Type: "int64"}}},
		"GetOut": {Fields: map[string]rpcschema.Field{"Total": {Number: 1, Type: "int64"}}},
	},
}

// shopApp writes an app named shop with no pages that holds the snapshot of billing and whose
// main.go calls billing.Get.
func shopApp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/shop\n\ngo 1.26.0\n")
	write(t, dir, "aicoded.yaml", "app: shop\n")
	write(t, dir, ".aicoded/services/billing.json", string(heldBilling.Marshal()))
	write(t, dir, "main.go", `package main

import (
	"context"

	"example.com/shop/services/billing"
)

func main() {
	_, _ = billing.Get(context.Background(), billing.GetIn{ID: 1})
}
`)
	return dir
}

func TestGenerateCaller(t *testing.T) {
	dir := shopApp(t)
	require.NoError(t, Generate(dir))
	assert.Contains(t, read(t, dir, "services/billing/client_gen.go"), "func Get(ctx context.Context, in GetIn) (GetOut, error) {")
	assert.Equal(t, "app: shop\n"+accessMarker+"\naccess: {}\n"+servicesMarker+`
services:
  calls:
    "billing": ["Get"]
`, read(t, dir, "aicoded.yaml"), "Ping is held but not called")

	require.NoError(t, os.Remove(filepath.Join(dir, ".aicoded", "services", "billing.json")))
	require.NoError(t, Generate(dir))
	assert.NoDirExists(t, filepath.Join(dir, "services"), "the client of an app no longer held is removed with its emptied folders")
	assert.Equal(t, "app: shop\n"+accessMarker+"\naccess: {}\n"+servicesMarker+"\nservices: {}\n", read(t, dir, "aicoded.yaml"))
}

func TestGenerateKeepsFoldersWithOtherFiles(t *testing.T) {
	dir := shopApp(t)
	require.NoError(t, Generate(dir))
	write(t, dir, "services/billing/notes.go", "package billing\n")
	require.NoError(t, os.Remove(filepath.Join(dir, ".aicoded", "services", "billing.json")))
	require.NoError(t, Generate(dir))
	assert.NoFileExists(t, filepath.Join(dir, "services", "billing", "client_gen.go"))
	assert.FileExists(t, filepath.Join(dir, "services", "billing", "notes.go"))
}

func TestGenerateRefusesHeldSnapshots(t *testing.T) {
	dir := shopApp(t)
	held := filepath.Join(dir, ".aicoded", "services")
	require.NoError(t, os.Rename(filepath.Join(held, "billing.json"), filepath.Join(held, "invoices.json")))
	assert.Equal(t, "E-RPC-005", errs.Code(Generate(dir)), "the file is named after another app")

	dir = shopApp(t)
	for _, app := range []string{"a-b", "ab"} {
		s := heldBilling
		s.App = app
		write(t, dir, ".aicoded/services/"+app+".json", string(s.Marshal()))
	}
	assert.Equal(t, "E-RPC-006", errs.Code(Generate(dir)), "both clients would be the package ab")
	assert.NoFileExists(t, filepath.Join(dir, "services", "billing", "client_gen.go"), "nothing is written")
}
