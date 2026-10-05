package rpcgen

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/rpcschema"
)

func TestCalls(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "pages/dataprovider.go", `package pages

import (
	"context"

	bill "example.com/shop/services/billing"
	"example.com/shop/services/reports"
)

var refund = bill.Refund

func load(ctx context.Context) error {
	_, err := bill.GetInvoice(ctx, bill.GetInvoiceIn{})
	_ = reports.Version
	return err
}
`)
	write(t, dir, "pages/dot.go", "package pages\n\nimport . \"example.com/shop/services/billing\"\n\nvar _ = Ping\n")
	for _, rel := range []string{"main_test.go", "pages/route_gen.go", "services/billing/extra.go", "testdata/a.go", ".git/a.go",
		"_old/a.go", "vendor/a.go", "node_modules/a.go", "tools/a.go"} {
		write(t, dir, rel, "package x\n\nimport \"example.com/shop/services/billing\"\n\nvar _ = billing.Ping\n")
	}
	write(t, dir, "tools/go.mod", "module example.com/tools\n")
	held := map[string]rpcschema.Schema{
		"billing": {App: "billing", Methods: map[string]rpcschema.Method{"GetInvoice": {}, "Ping": {}, "Refund": {}}},
		"reports": {App: "reports", Methods: map[string]rpcschema.Method{"Monthly": {}}},
	}
	calls, err := Calls(dir, "example.com/shop", held)
	require.NoError(t, err)
	assert.Equal(t, map[string][]string{"billing": {"GetInvoice", "Refund"}}, calls,
		"a function value counts, a type and a dot import do not, and tests, generated code, clients and the folders Go skips are not read, nor are other modules")
}

func TestCallsCountsNamesThatShadowTheClient(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "main.go", `package main

import "example.com/shop/services/billing"

type client struct{}

func (client) Refund() {}

var _ billing.RefundIn

func main() {
	billing := client{}
	billing.Refund()
}
`)
	held := map[string]rpcschema.Schema{"billing": {App: "billing", Methods: map[string]rpcschema.Method{"Refund": {}}}}
	calls, err := Calls(dir, "example.com/shop", held)
	require.NoError(t, err)
	assert.Equal(t, map[string][]string{"billing": {"Refund"}}, calls,
		"the scan is syntactic: a local named like the client counts, which only widens services.calls")
}

func TestCallsReadsABrokenFileAsFarAsItParses(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "main.go", "package main\n\nimport \"example.com/shop/services/billing\"\n\nvar _ = billing.Ping\n\nfunc main() {\n\tbroken(\n")
	held := map[string]rpcschema.Schema{"billing": {App: "billing", Methods: map[string]rpcschema.Method{"Ping": {}}}}
	calls, err := Calls(dir, "example.com/shop", held)
	require.NoError(t, err, "the Go build reports the syntax error, as E-CHK-002")
	assert.Equal(t, map[string][]string{"billing": {"Ping"}}, calls)
}
