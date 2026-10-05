package s_id

import "aicoded.dev/framework/cmd/aicoded/internal/devapi"

// Message is one message as the page shows it.
type Message struct {
	App     string
	ID      string
	Folder  string
	Stamp   string // the time as RFC 3339
	Time    string
	From    string
	To      string
	Cc      string
	Bcc     string
	ReplyTo string
	Subject string
	Text    string
	// HTML is whether the message has an HTML part, which the page shows in a sandboxed frame.
	HTML        bool
	Attachments []Attachment
}

// Attachment describes an attachment.
type Attachment = devapi.Attachment
