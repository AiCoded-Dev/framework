package n_id

import "site/deps"

// Note is a stored note.
type Note = deps.Note

// StarIn is what the page sends to star a note.
type StarIn struct {
	Starred bool `json:"starred"`
}

// StarOut is what the page gets back.
type StarOut struct {
	Starred bool   `json:"starred"`
	Title   string `json:"title"`
}
