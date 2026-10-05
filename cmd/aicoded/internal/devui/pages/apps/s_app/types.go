package s_app

import "aicoded.dev/framework/cmd/aicoded/internal/problem"

// App is what the page shows of the app.
type App struct {
	State     string
	Since     string
	Stamp     string // since, as RFC 3339
	URL       string
	Problems  []Problem
	Command   string
	RunnerDir string
}

// Problem is why the app failed.
type Problem = problem.Problem
