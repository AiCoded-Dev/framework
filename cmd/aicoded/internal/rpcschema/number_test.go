package rpcschema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// unnumbered returns what parsing billing's rpc package gives: types with field types only.
func unnumbered(types map[string]map[string]string) Schema {
	s := Schema{App: "billing", Methods: map[string]Method{"GetInvoice": {In: "GetInvoiceIn", Out: "Invoice"}}, Types: map[string]Type{}}
	for name, fields := range types {
		t := Type{Fields: map[string]Field{}}
		for f, typ := range fields {
			t.Fields[f] = Field{Type: typ}
		}
		s.Types[name] = t
	}
	return s
}

// numbers returns the field numbers of each type of s.
func numbers(s Schema) map[string]map[string]int {
	out := map[string]map[string]int{}
	for name, t := range s.Types {
		out[name] = map[string]int{}
		for f, x := range t.Fields {
			out[name][f] = x.Number
		}
	}
	return out
}

func TestNumberAFreshSchema(t *testing.T) {
	s := Number(nil, unnumbered(map[string]map[string]string{
		"GetInvoiceIn": {"ID": "int64"},
		"Invoice":      {"Total": "int64", "ID": "int64", "Lines": "[]Line"},
		"Line":         {"Amount": "int64"},
		"Unused":       {"X": "bool"},
	}))
	assert.Equal(t, map[string]map[string]int{
		"GetInvoiceIn": {"ID": 1},
		"Invoice":      {"ID": 1, "Lines": 2, "Total": 3},
		"Line":         {"Amount": 1},
	}, numbers(s), "numbered in name order; a type no function reaches is left out")
	assert.Equal(t, Field{3, "int64"}, s.Types["Invoice"].Fields["Total"])
	assert.Equal(t, "billing", s.App)
}

func TestNumberAgainstTheSnapshot(t *testing.T) {
	prev := billing()
	s := Number(&prev, unnumbered(map[string]map[string]string{
		"GetInvoiceIn": {"ID": "int64"},
		"Invoice":      {"ID": "string", "Lines": "[]Line", "Total": "int64"},
		"Line":         {"Amount": "int64", "Qty": "int32"},
	}))
	assert.Equal(t, map[string]map[string]int{
		"GetInvoiceIn": {"ID": 1},
		"Invoice":      {"ID": 1, "Lines": 3, "Total": 6},
		"Line":         {"Amount": 1, "Qty": 3},
	}, numbers(s), "numbers are kept, and a new one is one more than the highest ever used")
	assert.Equal(t, []int{2, 4, 5}, s.Types["Invoice"].Retired, "the number of the removed Paid is retired")
	assert.Equal(t, []int{2}, s.Types["Line"].Retired)

	again := Number(&s, unnumbered(map[string]map[string]string{
		"GetInvoiceIn": {"ID": "int64"},
		"Invoice":      {"ID": "string", "Lines": "[]Line", "Total": "int64", "Paid": "*time"},
		"Line":         {"Amount": "int64", "Qty": "int32"},
	}))
	assert.Equal(t, 7, again.Types["Invoice"].Fields["Paid"].Number, "a field added back gets a new number")

	dropped := Number(&s, unnumbered(map[string]map[string]string{
		"GetInvoiceIn": {"ID": "int64"},
		"Invoice":      {"ID": "string"},
		"Line":         {"Amount": "int64"},
	}))
	assert.NotContains(t, dropped.Types, "Line", "no function reaches Line")
}

func TestNumberSkipsTheReservedRange(t *testing.T) {
	prev := Schema{Types: map[string]Type{"GetInvoiceIn": {Retired: []int{18999}}}}
	s := Number(&prev, unnumbered(map[string]map[string]string{"GetInvoiceIn": {"ID": "int64"}, "Invoice": {}}))
	assert.Equal(t, 20000, s.Types["GetInvoiceIn"].Fields["ID"].Number)
}
