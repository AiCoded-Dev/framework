package s_app

// Message sums up one message as the page shows it.
type Message struct {
	ID      string
	Folder  string
	Stamp   string // the time as RFC 3339
	Time    string
	From    string
	To      string
	Subject string
}
