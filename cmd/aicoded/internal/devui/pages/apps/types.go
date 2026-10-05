package apps

// App is one row of the apps table.
type App struct {
	Name  string
	State string
	Since string
	Stamp string // since, as RFC 3339
	URL   string
	// Problem is the code of the app's first problem.
	Problem string
}
