package manifest_test

import (
	"fmt"

	"aicoded.dev/framework/manifest"
)

// Parse checks a permission list and returns its fields. A list with problems returns every
// problem, each with a code and its position.
func ExampleParse() {
	m, err := manifest.Parse(manifest.FileName, []byte("app: rooms\negress: [api.partner.example]\n"))
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(m.Egress)
	// Output: [api.partner.example]
}

// Diff lists what changed between two permission lists, field by field, as the platform's
// change records show it. A new order of a list is no change.
func ExampleDiff() {
	before := manifest.Manifest{
		App:      "rooms",
		Access:   map[string]manifest.Access{"/rooms/{id}": {Require: []string{"staff"}}},
		Settings: []string{"greeting", "report_day"},
	}
	after := manifest.Manifest{
		App:      "rooms",
		Access:   map[string]manifest.Access{"/rooms/{id}": {Require: []string{"staff"}, Guard: true}},
		Egress:   []string{"api.partner.example"},
		Settings: []string{"report_day", "greeting"},
	}
	for _, c := range manifest.Diff(before, after) {
		fmt.Printf("%+v\n", c)
	}
	// Output:
	// {Field:access Key:/rooms/{id} Before:require staff After:require staff; guard}
	// {Field:egress Key: Before: After:api.partner.example}
}
