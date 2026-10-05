package pages

// Key uses names that only generated code may use.
func Key() string {
	return routeKey + state{}.conn
}
