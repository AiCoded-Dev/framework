package rpcschema

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
)

// billing is the schema of testdata/billing.json; every call returns fresh maps.
func billing() Schema {
	return Schema{
		App:     "billing",
		Methods: map[string]Method{"GetInvoice": {In: "GetInvoiceIn", Out: "Invoice"}},
		Types: map[string]Type{
			"GetInvoiceIn": {Fields: map[string]Field{"ID": {1, "int64"}}},
			"Invoice": {Fields: map[string]Field{"ID": {1, "int64"}, "Lines": {3, "[]Line"}, "Paid": {4, "*time"}},
				Retired: []int{2, 5}},
			"Line": {Fields: map[string]Field{"Amount": {1, "int64"}, "Note": {2, "string"}}},
		},
	}
}

func TestLoad(t *testing.T) {
	s, err := Load(filepath.Join("testdata", "billing.json"))
	require.NoError(t, err)
	assert.Equal(t, billing(), s)
}

func TestMarshal(t *testing.T) {
	want, err := os.ReadFile(filepath.Join("testdata", "billing.json"))
	require.NoError(t, err)
	shuffled := billing()
	shuffled.Types["Invoice"] = Type{Fields: shuffled.Types["Invoice"].Fields, Retired: []int{5, 2}}
	assert.Equal(t, string(want), string(shuffled.Marshal()))
	assert.Equal(t, []int{5, 2}, shuffled.Types["Invoice"].Retired, "Marshal leaves its input alone")

	for _, s := range []Schema{billing(), {App: "billing"}} {
		got, err := Parse("billing.json", s.Marshal())
		require.NoError(t, err)
		assert.Equal(t, s.Marshal(), got.Marshal())
	}
}

func TestParseRefuses(t *testing.T) {
	good, err := os.ReadFile(filepath.Join("testdata", "billing.json"))
	require.NoError(t, err)
	for name, c := range map[string]struct {
		old, new string
		line     int
	}{
		"invalid JSON":        {`"out": "Invoice"`, `"out": "Invoice",`, 7},
		"unknown key":         {`"methods": {`, `"method": {`, 3},
		"unknown field key":   {`"number": 3,`, `"num": 3,`, 25},
		"repeated key":        {`"Note": {`, `"Amount": {`, 44},
		"bad app name":        {`"app": "billing"`, `"app": "Billing"`, 2},
		"bad function name":   {`"GetInvoice": {`, `"getInvoice": {`, 4},
		"unexported field":    {`"Note": {`, `"note": {`, 44},
		"slice of pointers":   {`"type": "[]Line"`, `"type": "[]*Line"`, 26},
		"pointer to bytes":    {`"type": "*time"`, `"type": "*bytes"`, 30},
		"missing field type":  {`"type": "[]Line"`, `"type": "[]Lines"`, 26},
		"unknown scalar":      {`"type": "string"`, `"type": "int"`, 46},
		"input not a type":    {`"in": "GetInvoiceIn"`, `"in": "GetInvoiceRequest"`, 5},
		"output a scalar":     {`"out": "Invoice"`, `"out": "string"`, 6},
		"number used twice":   {`"number": 4,`, `"number": 3,`, 29},
		"number zero":         {`"number": 1,`, `"number": 0,`, 13},
		"reserved number":     {`"number": 4,`, `"number": 19000,`, 29},
		"fractional number":   {`"number": 4,`, `"number": 4.5,`, 29},
		"number also retired": {`"number": 3,`, `"number": 5,`, 25},
		"retired twice":       {"        2,\n", "        5,\n", 35},
		"data after the end":  {"}\n}\n", "}\n}\n{}\n", 52},
		"no app":              {`"app": "billing",`, ``, 1},
	} {
		t.Run(name, func(t *testing.T) {
			require.Contains(t, string(good), c.old)
			_, err := Parse("billing.json", []byte(strings.Replace(string(good), c.old, c.new, 1)))
			var e *errs.Error
			require.ErrorAs(t, err, &e)
			assert.Equal(t, "E-RPC-005", e.Code)
			assert.Equal(t, fmt.Sprintf("billing.json:%d", c.line), e.Pos, e.Msg)
		})
	}

	_, err = Parse("billing.json", good[:len(good)/2])
	assert.Equal(t, "E-RPC-005", errs.Code(err), "cut short")
	_, err = Load(filepath.Join("testdata", "missing.json"))
	require.ErrorIs(t, err, fs.ErrNotExist)
}
