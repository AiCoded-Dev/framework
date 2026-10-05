package rpcadd_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/rpcadd"
	"aicoded.dev/framework/internal/errs"
)

// app writes the app name into root/name with the files by path.
func app(t *testing.T, root, name string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	files["go.mod"] = "module example.com/" + name + "\n\ngo 1.25.0\n"
	files["aicoded.yaml"] = "app: " + name + "\n"
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	}
	return dir
}

const billingRPC = `package rpc

import "context"

type GetIn struct{ ID int64 }

type GetOut struct{ Total int64 }

//ssr:access caller=shop role=editor
func Get(ctx context.Context, in GetIn) (GetOut, error) { return GetOut{}, nil }
`

func TestAdd(t *testing.T) {
	root := t.TempDir()
	billing := app(t, root, "billing", map[string]string{"rpc/billing.go": billingRPC})
	shop := app(t, root, "shop", map[string]string{})

	var out bytes.Buffer
	require.NoError(t, rpcadd.Add(shop, "billing", &out))
	assert.Equal(t, "aicoded rpc add: call the functions of billing through the package in services/billing\n", out.String())
	assert.FileExists(t, filepath.Join(billing, "rpc", "server_gen.go"), "the called app is generated first")
	snapshot, err := os.ReadFile(filepath.Join(billing, ".aicoded", "rpc.json"))
	require.NoError(t, err)
	held, err := os.ReadFile(filepath.Join(shop, ".aicoded", "services", "billing.json"))
	require.NoError(t, err)
	assert.Equal(t, string(snapshot), string(held))
	assert.FileExists(t, filepath.Join(shop, "services", "billing", "client_gen.go"))
}

func TestAddRefuses(t *testing.T) {
	root := t.TempDir()
	app(t, root, "billing", map[string]string{"main.go": "package main\n"})
	app(t, root, "reports", map[string]string{"rpc/reports.go": "package rpc\n"})
	shop := app(t, root, "shop", map[string]string{"rpc/shop.go": billingRPC})
	for name, callee := range map[string]string{
		"an app with no rpc/":            "billing",
		"an app that serves no function": "reports",
		"an app that is not there":       "stock",
		"the app itself":                 "shop",
	} {
		err := rpcadd.Add(shop, callee, &bytes.Buffer{})
		assert.Equal(t, "E-RPC-007", errs.Code(err), name)
	}
	err := rpcadd.Add(shop, "../billing", &bytes.Buffer{})
	assert.Equal(t, "E-RPC-006", errs.Code(err), "a name that is not an app name never reaches a path")
	var e *errs.Error
	require.ErrorAs(t, err, &e)
	assert.Empty(t, e.Pos, "the app is not held, so no file is to blame")
	assert.NoDirExists(t, filepath.Join(shop, ".aicoded"), "the calling app is not generated")
	assert.NoDirExists(t, filepath.Join(shop, "services"))
}

func TestAddRefusesAClashingPackageName(t *testing.T) {
	root := t.TempDir()
	app(t, root, "ab", map[string]string{"rpc/ab.go": billingRPC})
	held := `{"app": "a-b", "methods": {}, "types": {}}` + "\n"
	shop := app(t, root, "shop", map[string]string{".aicoded/services/a-b.json": held})

	err := rpcadd.Add(shop, "ab", &bytes.Buffer{})
	assert.Equal(t, "E-RPC-006", errs.Code(err), "the clients of a-b and ab would both be the package ab")
	assert.NoFileExists(t, filepath.Join(shop, ".aicoded", "services", "ab.json"), "nothing is written, so aicoded generate still runs")
	assert.NoDirExists(t, filepath.Join(root, "ab", ".aicoded"), "the called app is not generated either")
}

func TestAddRefusesTwoAppsWithOneName(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	app(t, root, "billing", map[string]string{"rpc/billing.go": billingRPC})
	reports := app(t, root, "reports", map[string]string{})
	other := filepath.Join(root, "reports-copy")
	require.NoError(t, os.CopyFS(other, os.DirFS(reports)))
	shop := app(t, root, "shop", map[string]string{})

	err = rpcadd.Add(shop, "billing", &bytes.Buffer{})
	var e *errs.Error
	require.ErrorAs(t, err, &e)
	assert.Equal(t, "E-DEV-010", e.Code)
	assert.Equal(t, "the apps in "+reports+" and "+other+" are both named reports", e.Msg)
	assert.NoDirExists(t, filepath.Join(shop, ".aicoded"), "nothing is written")
}

func TestAddAgainAfterABreak(t *testing.T) {
	root := t.TempDir()
	ping := "\n//ssr:access caller=shop role=editor\nfunc Ping(ctx context.Context, in GetIn) (GetIn, error) { return in, nil }\n"
	billing := app(t, root, "billing", map[string]string{"rpc/billing.go": billingRPC + ping})
	shop := app(t, root, "shop", map[string]string{})
	require.NoError(t, rpcadd.Add(shop, "billing", &bytes.Buffer{}))
	held := filepath.Join(shop, ".aicoded", "services", "billing.json")
	old, err := os.ReadFile(held)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(billing, "rpc", "billing.go"), []byte(billingRPC), 0o600))
	reports := app(t, root, "reports", map[string]string{".aicoded/services/billing.json": string(old)})
	err = rpcadd.Add(shop, "billing", &bytes.Buffer{})
	var e *errs.Error
	require.ErrorAs(t, err, &e)
	assert.Equal(t, "E-RPC-013", e.Code, "reports still calls Ping")
	assert.True(t, strings.HasPrefix(e.Pos, filepath.Join(reports, ".aicoded", "services", "billing.json")+":"), e.Pos)
	kept, err := os.ReadFile(held)
	require.NoError(t, err)
	assert.Equal(t, string(old), string(kept), "shop keeps its snapshot")

	require.NoError(t, os.RemoveAll(reports))
	require.NoError(t, rpcadd.Add(shop, "billing", &bytes.Buffer{}), "the caller's own snapshot is the one being replaced")
	snapshot, err := os.ReadFile(filepath.Join(billing, ".aicoded", "rpc.json"))
	require.NoError(t, err)
	fresh, err := os.ReadFile(held)
	require.NoError(t, err)
	assert.Equal(t, string(snapshot), string(fresh))
	assert.NotContains(t, string(fresh), "Ping")
}
