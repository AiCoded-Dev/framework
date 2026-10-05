// Package deps is the test site's in-memory store.
package deps

import (
	"slices"
	"sync"
)

// Note is a note owned by the viewer who wrote it.
type Note struct {
	ID       int64
	Title    string
	Priority int
	Owner    string
	Starred  bool
}

// Deps holds the site's notes.
type Deps struct {
	mu    sync.Mutex
	notes []Note
}

// New returns an empty store.
func New() *Deps { return &Deps{} }

// Add stores n and returns its id.
func (d *Deps) Add(n Note) int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	n.ID = int64(len(d.notes)) + 1
	d.notes = append(d.notes, n)
	return n.ID
}

// Get returns the note with id.
func (d *Deps) Get(id int64) (Note, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if id < 1 || id > int64(len(d.notes)) {
		return Note{}, false
	}
	return d.notes[id-1], true
}

// Owned returns the notes of owner.
func (d *Deps) Owned(owner string) []Note {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.DeleteFunc(slices.Clone(d.notes), func(n Note) bool { return n.Owner != owner })
}

// Star marks or unmarks the note with id.
func (d *Deps) Star(id int64, on bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if id >= 1 && id <= int64(len(d.notes)) {
		d.notes[id-1].Starred = on
	}
}

// Give makes owner the owner of the note with id.
func (d *Deps) Give(id int64, owner string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if id >= 1 && id <= int64(len(d.notes)) {
		d.notes[id-1].Owner = owner
	}
}

// Rename changes the title of the note with id.
func (d *Deps) Rename(id int64, title string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if id >= 1 && id <= int64(len(d.notes)) {
		d.notes[id-1].Title = title
	}
}
