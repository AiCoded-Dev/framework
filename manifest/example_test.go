package manifest_test

import (
	"fmt"

	"aicoded.dev/framework/manifest"
)

// Parse checks a permission list and returns its fields. A list with problems returns every
// problem, each with a code and its position.
func ExampleParse() {
	m, err := manifest.Parse(manifest.FileName, []byte(`app: rooms
audience: {internal: [everyone]}
egress:
  - hooks.partner.example
  - {host: api.partner.example, secret: partner_api_key, header: Authorization, prefix: "Bearer "}
secrets: [partner_api_key]
`))
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(m.EgressHosts())
	b, _ := m.Binding("api.partner.example")
	fmt.Printf("%s in %s after %q\n", b.Secret, b.Header, b.Prefix)
	// Output:
	// [api.partner.example hooks.partner.example]
	// partner_api_key in Authorization after "Bearer "
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
		Egress:   []manifest.Egress{{Host: "api.partner.example"}},
		Settings: []string{"report_day", "greeting"},
	}
	for _, c := range manifest.Diff(before, after) {
		fmt.Printf("%+v\n", c)
	}
	// Output:
	// {Field:access Key:/rooms/{id} Before:require staff After:require staff; guard}
	// {Field:egress Key: Before: After:api.partner.example}
}
