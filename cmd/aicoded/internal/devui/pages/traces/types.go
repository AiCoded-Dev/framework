package traces

import "aicoded.dev/framework/cmd/aicoded/internal/devapi"

// Query is what the page lists.
type Query = devapi.TraceQuery

// Traces is what the page shows of the traces that match its query.
type Traces struct {
	// Traces are the newest traces, newest first.
	Traces []Trace
	// Problem is why the traces could not be listed.
	Problem string
}

// Trace sums up one trace as the page shows it.
type Trace struct {
	ID       string
	Stamp    string // the start as RFC 3339
	Start    string
	Root     string
	Apps     string
	Spans    int
	Duration string
	Error    bool
}
