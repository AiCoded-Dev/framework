package personas

// Persona is one persona, with its roles and groups joined into text.
type Persona struct {
	Name   string
	Roles  string
	Groups string
}

// Switcher is the page of an app where the developer chooses the persona that app sees.
type Switcher struct {
	App string
	URL string
}
