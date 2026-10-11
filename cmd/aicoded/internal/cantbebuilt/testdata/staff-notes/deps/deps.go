// Package deps holds what the pages share: the notes, kept in memory.
package deps

import "sync"

// Deps keeps the notes added since the app started, oldest first.
type Deps struct {
	mu    sync.Mutex
	notes []string
}

// Add stores a note.
func (d *Deps) Add(note string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.notes = append(d.notes, note)
}

// Notes returns the notes added so far.
func (d *Deps) Notes() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.notes...)
}
