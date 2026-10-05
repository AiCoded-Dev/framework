package form_test

import (
	"fmt"
	"slices"

	"aicoded.dev/framework/web/form"
)

// Init<Form> sets the options of a select, alone or in groups. A viewer can post only the value
// of an enabled option; any other value gets form.MessageInvalidOption.
func ExampleSelect_SetOptions() {
	var team form.Select[string] // a field of the generated Form<Name>Values
	team.SetOptions([]form.SelectOptionElement[string]{
		form.SelectOption[string]{Value: "ops", Label: "Operations"},
		form.SelectOptionGroup[string]{Label: "Sales", Options: []form.SelectOptionElement[string]{
			form.SelectOption[string]{Value: "sales-eu", Label: "Europe"},
			form.SelectOption[string]{Value: "sales-us", Label: "Americas", Disabled: true},
		}},
	})
	for _, v := range []string{"ops", "sales-eu", "sales-us", "hr"} {
		offered := slices.ContainsFunc(team.GetOptions(), func(o form.SelectOptionElement[string]) bool { return o.Allows(v) })
		fmt.Println(v, offered)
	}
	// Output:
	// ops true
	// sales-eu true
	// sales-us false
	// hr false
}

// Process<Form> runs only when every field is valid, so it checks what only the server knows and
// sets the error that the page shows next to the field.
func ExampleInput_SetError() {
	taken := map[string]bool{"alice": true, "bob": true}
	var login form.Input[string] // a field of the generated Form<Name>Values
	login.SetValue("alice")      // as the viewer posted it

	if taken[login.GetValue()] {
		login.SetError("This login is taken.")
	}
	fmt.Println(login.HasError(), login.GetError())
	// Output: true This login is taken.
}
