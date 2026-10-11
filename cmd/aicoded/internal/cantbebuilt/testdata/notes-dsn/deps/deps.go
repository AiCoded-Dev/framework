// Package deps holds what the pages share: the notes database.
package deps

import (
	"context"
	"database/sql"
	"errors"
)

// Note is one note in the notes table.
type Note struct {
	ID int64
	// Owner is the login of the person who wrote it.
	Owner string
	Title string
	Body  string
}

// ErrNoNote is the error of an id no note has.
var ErrNoNote = errors.New("no such note")

// Deps is filled by Start before the app serves.
type Deps struct{ DB *sql.DB }

// Start opens the database and creates the notes table. An owner compares exactly, with no case
// folding and no trailing-space padding.
func (d *Deps) Start(ctx context.Context) error {
	db, err := sql.Open("mysql", "notes:hunter2@tcp(db.internal:3306)/notes")
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS notes (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		owner VARCHAR(512) COLLATE utf8mb4_0900_bin NOT NULL,
		title VARCHAR(100) NOT NULL,
		body TEXT NOT NULL,
		KEY (owner)
	)`); err != nil {
		return err
	}
	d.DB = db
	return nil
}

// NotesOf returns the notes of owner, newest first.
func (d *Deps) NotesOf(ctx context.Context, owner string) ([]Note, error) {
	rows, err := d.DB.QueryContext(ctx, "SELECT id, owner, title, body FROM notes WHERE owner = ? ORDER BY id DESC", owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var notes []Note
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.Owner, &n.Title, &n.Body); err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

// Note returns the note with this id, or ErrNoNote.
func (d *Deps) Note(ctx context.Context, id int64) (Note, error) {
	var n Note
	err := d.DB.QueryRowContext(ctx, "SELECT id, owner, title, body FROM notes WHERE id = ?", id).Scan(&n.ID, &n.Owner, &n.Title, &n.Body)
	if errors.Is(err, sql.ErrNoRows) {
		return Note{}, ErrNoNote
	}
	return n, err
}

// Add stores a note written by owner.
func (d *Deps) Add(ctx context.Context, owner, title, body string) error {
	_, err := d.DB.ExecContext(ctx, "INSERT INTO notes (owner, title, body) VALUES (?, ?, ?)", owner, title, body)
	return err
}
