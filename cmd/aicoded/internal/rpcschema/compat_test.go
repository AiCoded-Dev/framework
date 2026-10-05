package rpcschema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompatible(t *testing.T) {
	for name, c := range map[string]struct {
		change func(s Schema)
		want   []Break
	}{
		"nothing changed": {func(Schema) {}, nil},
		"added field and function": {func(s Schema) {
			s.Types["Invoice"].Fields["Due"] = Field{6, "time"}
			s.Methods["ListInvoices"] = Method{In: "GetInvoiceIn", Out: "Invoice"}
		}, nil},
		"renamed type": {func(s Schema) {
			s.Types["InvoiceLine"] = s.Types["Line"]
			delete(s.Types, "Line")
			s.Types["Invoice"].Fields["Lines"] = Field{3, "[]InvoiceLine"}
		}, nil},
		"removed function": {func(s Schema) { delete(s.Methods, "GetInvoice") },
			[]Break{{"GetInvoice", "", "function removed"}}},
		"removed field": {func(s Schema) { delete(s.Types["Invoice"].Fields, "Paid") },
			[]Break{{"GetInvoice", "Invoice.Paid", "removed"}}},
		"renamed field": {func(s Schema) {
			delete(s.Types["Invoice"].Fields, "Paid")
			s.Types["Invoice"].Fields["PaidAt"] = Field{4, "*time"}
		}, []Break{{"GetInvoice", "Invoice.Paid", "renamed to PaidAt"}}},
		"renumbered field": {func(s Schema) { s.Types["Invoice"].Fields["Lines"] = Field{6, "[]Line"} },
			[]Break{{"GetInvoice", "Invoice.Lines", "number changed from 3 to 6"}}},
		"changed scalar in a nested type": {func(s Schema) { s.Types["Line"].Fields["Amount"] = Field{1, "string"} },
			[]Break{{"GetInvoice", "Invoice.Lines[].Amount", "type changed from int64 to string"}}},
		"T to []T": {func(s Schema) { s.Types["GetInvoiceIn"].Fields["ID"] = Field{1, "[]int64"} },
			[]Break{{"GetInvoice", "GetInvoiceIn.ID", "type changed from int64 to []int64"}}},
		"*T to T": {func(s Schema) { s.Types["Invoice"].Fields["Paid"] = Field{4, "time"} },
			[]Break{{"GetInvoice", "Invoice.Paid", "type changed from *time to time"}}},
		"struct to scalar": {func(s Schema) { s.Types["Invoice"].Fields["Lines"] = Field{3, "[]string"} },
			[]Break{{"GetInvoice", "Invoice.Lines", "type changed from []Line to []string"}}},
	} {
		t.Run(name, func(t *testing.T) {
			current := billing()
			c.change(current)
			assert.Equal(t, c.want, Compatible(billing(), current))
		})
	}
}

func TestCompatibleSortsBreaks(t *testing.T) {
	current := billing()
	current.Methods["Void"] = Method{In: "GetInvoiceIn", Out: "Invoice"}
	held := billing()
	held.Methods["Void"] = Method{In: "GetInvoiceIn", Out: "Invoice"}
	held.Methods["Archive"] = Method{In: "GetInvoiceIn", Out: "Invoice"}
	delete(current.Types["Invoice"].Fields, "Paid")
	delete(current.Types["Invoice"].Fields, "ID")
	assert.Equal(t, []Break{
		{"Archive", "", "function removed"},
		{"GetInvoice", "Invoice.ID", "removed"},
		{"GetInvoice", "Invoice.Paid", "removed"},
		{"Void", "Invoice.ID", "removed"},
		{"Void", "Invoice.Paid", "removed"},
	}, Compatible(held, current))
}

func TestCompatibleRecursiveTypes(t *testing.T) {
	tree := func() Schema {
		return Schema{App: "docs", Methods: map[string]Method{"Get": {In: "Node", Out: "Node"}}, Types: map[string]Type{
			"Node": {Fields: map[string]Field{"Name": {1, "string"}, "Next": {2, "*Node"}, "Children": {3, "[]Node"}}},
		}}
	}
	assert.Empty(t, Compatible(tree(), tree()))
	current := tree()
	current.Types["Node"].Fields["Name"] = Field{1, "bytes"}
	assert.Equal(t, []Break{{"Get", "Node.Name", "type changed from string to bytes"}}, Compatible(tree(), current))
}
