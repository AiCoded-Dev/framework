package rpcgen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/rpcschema"
	"aicoded.dev/framework/internal/errs"
)

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
}

// header starts rpc/a.go in the tests below; the source after it begins on line 10.
const header = `package rpc

import (
	"context"
	"time"
)

type In struct{}
type Out struct{}
`

// parseErr parses an app whose rpc/a.go is header followed by src and returns the position and
// code of the first problem.
func parseErr(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "rpc/a.go", header+src)
	_, ok, err := Parse(dir)
	assert.True(t, ok)
	var e *errs.Error
	require.ErrorAs(t, err, &e, src)
	return e.Pos + " " + e.Code
}

func TestParseRefuses(t *testing.T) {
	const get = "func Get(ctx context.Context, in In) (Out, error) { return Out{}, nil }\n"
	for name, c := range map[string]struct{ src, want string }{
		"missing directive":     {get, "rpc/a.go:10 E-RPC-001"},
		"two directives":        {"//ssr:access caller=shop role=x\n//ssr:access caller=shop role=y\n" + get, "rpc/a.go:11 E-RPC-002"},
		"unknown attribute":     {"//ssr:access caller=shop role=x mode=fast\n" + get, "rpc/a.go:10 E-RPC-002"},
		"repeated attribute":    {"//ssr:access caller=shop caller=bill role=x\n" + get, "rpc/a.go:10 E-RPC-002"},
		"no caller":             {"//ssr:access role=x\n" + get, "rpc/a.go:10 E-RPC-002"},
		"bad app name":          {"//ssr:access caller=Shop role=x\n" + get, "rpc/a.go:10 E-RPC-002"},
		"bad role":              {"//ssr:access caller=shop role=Admin\n" + get, "rpc/a.go:10 E-RPC-002"},
		"neither role nor apps": {"//ssr:access caller=shop\n" + get, "rpc/a.go:10 E-RPC-002"},
		"wrong arity": {"//ssr:access caller=shop role=x\nfunc Get(in In) (Out, error) { return Out{}, nil }\n",
			"rpc/a.go:11 E-RPC-003"},
		"non-struct input": {"type ID int64\n\n//ssr:access caller=shop role=x\nfunc Get(ctx context.Context, in ID) (Out, error) { return Out{}, nil }\n",
			"rpc/a.go:13 E-RPC-003"},
		"input of another package": {"//ssr:access caller=shop role=x\nfunc Get(ctx context.Context, in time.Time) (Out, error) { return Out{}, nil }\n",
			"rpc/a.go:11 E-RPC-003"},
		"generated method name": {"func (x *In) aicodedDecode(b []byte) error { return nil }\n", "rpc/a.go:10 E-RPC-003"},
		"generated name":        {"var Server = 1\n", "rpc/a.go:10 E-RPC-003"},
		"predeclared type name": {"type string struct{}\n", "rpc/a.go:10 E-RPC-003"},
	} {
		assert.Equal(t, c.want, parseErr(t, c.src), name)
	}
}

func TestParseRefusesLinesThatDoNothing(t *testing.T) {
	const line, spaced = "//ssr:access caller=shop role=x\n", "// ssr:access caller=shop role=x\n"
	const get = "func Get(ctx context.Context, in In) (Out, error) { return Out{}, nil }\n"
	for name, c := range map[string]struct{ src, pos, msg string }{
		"on a method":               {line + "func (In) Get(ctx context.Context, in In) (Out, error) { return Out{}, nil }\n", "rpc/a.go:10", "no effect on a method"},
		"on an unexported function": {line + "func get(ctx context.Context, in In) (Out, error) { return Out{}, nil }\n", "rpc/a.go:10", "no effect on an unexported function"},
		"on a type":                 {line + "type Thing struct{}\n", "rpc/a.go:10", "no effect on a type"},
		"on a type in a group":      {"type (\n\t" + line + "\tThing struct{}\n)\n", "rpc/a.go:11", "no effect on a type"},
		"on a var":                  {line + "var limit = 3\n", "rpc/a.go:10", "no effect on a var"},
		"on a const":                {line + "const Limit = 3\n", "rpc/a.go:10", "no effect on a const"},
		"spaced":                    {spaced + get, "rpc/a.go:10", "write //ssr:access without a space"},
		"spaced with a tab":         {"//\tssr:access caller=shop role=x\n" + get, "rpc/a.go:10", "write //ssr:access without a space"},
		"spaced next to a line":     {line + spaced + get, "rpc/a.go:11", "write //ssr:access without a space"},
		"spaced on a type":          {spaced + "type Thing struct{}\n", "rpc/a.go:10", "write //ssr:access without a space"},
	} {
		dir := t.TempDir()
		write(t, dir, "rpc/a.go", header+c.src)
		_, _, err := Parse(dir)
		var e *errs.Error
		if assert.ErrorAs(t, err, &e, name) {
			assert.Equal(t, "E-RPC-002", e.Code, name)
			assert.Equal(t, c.pos, e.Pos, name)
			assert.Contains(t, e.Msg, c.msg, name)
			assert.NotContains(t, err.Error(), "E-RPC-001", "%s: one error for the line", name)
		}
	}
}

func TestParseRefusesFields(t *testing.T) {
	for typ, want := range map[string]string{
		"F int":               "rpc/a.go:11 E-RPC-004",
		"F map[string]string": "rpc/a.go:11 E-RPC-004",
		"F []*In":             "rpc/a.go:11 E-RPC-004",
		"F *[]In":             "rpc/a.go:11 E-RPC-004",
		"F time.Duration":     "rpc/a.go:11 E-RPC-004",
		"In":                  "rpc/a.go:11 E-RPC-004",
		"f int32":             "rpc/a.go:11 E-RPC-004",
	} {
		src := "type Bad struct {\n\t" + typ + "\n}\n\n//ssr:access caller=shop role=x\nfunc Get(ctx context.Context, in Bad) (Out, error) { return Out{}, nil }\n"
		assert.Equal(t, want, parseErr(t, src), typ)
	}
}

func TestParseReportsEveryProblem(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "rpc/a.go", header+"func Get(ctx context.Context, in Bad) (Out, error) { return Out{}, nil }\n\ntype Bad struct{ F int }\n")
	_, _, err := Parse(dir)
	require.ErrorContains(t, err, "rpc/a.go:10: E-RPC-001")
	require.ErrorContains(t, err, "rpc/a.go:12: E-RPC-004")
	assert.Equal(t, "E-RPC-001", errs.Code(err))
}

func TestParseRefusesPackageName(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "rpc/a.go", "package billing\n")
	_, _, err := Parse(dir)
	require.ErrorContains(t, err, "rpc/a.go:1: E-RPC-003: the package in rpc/ is named billing")
}

func TestParseStopsOnSyntaxErrors(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "rpc/a.go", "package rpc\n\ntype In struct {\n")
	_, ok, err := Parse(dir)
	assert.True(t, ok)
	require.ErrorContains(t, err, "rpc/a.go:3:18: expected")
}

func TestParseWithoutRPC(t *testing.T) {
	_, ok, err := Parse(t.TempDir())
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestParse(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "rpc/invoice.go", `package rpc

import (
	ctx "context"
	clock "time"

	frameworkrpc "aicoded.dev/framework/rpc"
)

// Invoice is a bill and its lines; a type needs no ssr:access line.
type Invoice struct {
	ID     int64
	Lines  []Line
	Due    clock.Time
	Note   *string
	PDF    []byte
	Parent *Invoice
	Tags   []string
	Files  [][]byte
	Paid   *clock.Time
}

type Line struct {
	Amount float32
	Sub    []Line
}

type GetIn struct{ ID uint64 }

type PingOut struct{}

type Unused struct{ N int }

// GetInvoice returns an invoice.
//
//ssr:access caller=shop,reports role=editor|admin apps=true
func GetInvoice(c ctx.Context, in GetIn) (Invoice, error) { return Invoice{}, nil }

//ssr:access caller=shop role=*
func Ping(c ctx.Context, in GetIn) (PingOut, error) {
	return PingOut{}, frameworkrpc.Error(frameworkrpc.NotFound, "no ping")
}

func helper() {}

func (i *Invoice) Total() float64 { return 0 }
`)
	write(t, dir, "rpc/invoice_test.go", "package rpc\n\nfunc Untested() {}\n")
	write(t, dir, "rpc/server_gen.go", "package rpc\n\nfunc Server() {}\n")

	p, ok, err := Parse(dir)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, []Func{
		{Name: "GetInvoice", In: "GetIn", Out: "Invoice", Pos: "rpc/invoice.go:37",
			Access: Access{Callers: []string{"reports", "shop"}, Require: []string{"admin|editor"}, Apps: true}},
		{Name: "Ping", In: "GetIn", Out: "PingOut", Pos: "rpc/invoice.go:40",
			Access: Access{Callers: []string{"shop"}, Require: []string{"*"}}},
	}, p.Funcs)
	type f = rpcschema.Field
	assert.Equal(t, rpcschema.Schema{
		Methods: map[string]rpcschema.Method{"GetInvoice": {In: "GetIn", Out: "Invoice"}, "Ping": {In: "GetIn", Out: "PingOut"}},
		Types: map[string]rpcschema.Type{
			"GetIn":   {Fields: map[string]f{"ID": {Type: "uint64"}}},
			"PingOut": {Fields: map[string]f{}},
			"Invoice": {Fields: map[string]f{
				"ID": {Type: "int64"}, "Lines": {Type: "[]Line"}, "Due": {Type: "time"}, "Note": {Type: "*string"},
				"PDF": {Type: "bytes"}, "Parent": {Type: "*Invoice"}, "Tags": {Type: "[]string"}, "Files": {Type: "[]bytes"},
				"Paid": {Type: "*time"},
			}},
			"Line": {Fields: map[string]f{"Amount": {Type: "float32"}, "Sub": {Type: "[]Line"}}},
		},
	}, p.Schema, "the unused type is left out")
}

func TestDiagNeedsAFixLine(t *testing.T) {
	assert.Panics(t, func() { _ = diag("rpc/a.go:1", "E-RPC-999", "x") })
}
