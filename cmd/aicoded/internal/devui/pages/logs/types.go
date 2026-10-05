package logs

import "aicoded.dev/framework/cmd/aicoded/internal/devapi"

// Query is what the page lists.
type Query = devapi.LogQuery

// Logs is what the page shows of the entries that match its query.
type Logs struct {
	// Entries are the newest entries, newest first.
	Entries []Entry
	// Manual names the apps of the query that run by hand; their lines never reach aicoded dev.
	Manual []string
	// Problem is why the entries could not be listed.
	Problem string
}

// Entry is one log entry as the page shows it.
type Entry struct {
	Stamp   string // the time as RFC 3339
	Time    string
	App     string
	Source  string
	Level   string
	Class   string // level-<Level> for DEBUG, INFO, WARN and ERROR; "" for any other level
	Message string
	Attrs   string
	Trace   string // the trace id; "" when the entry has none, or one that is not a trace id
}
