// Package deps holds what the pages share: the notes database.
package deps

import (
	"context"
	"database/sql"

	"aicoded.dev/framework/sqldb"
)

// Note is one note in the notes table.
type Note struct {
	ID int64
	// Owner is the login of the person who wrote it.
	Owner string
	Title string
	Body  string
}

// Deps is filled by Start before the app serves.
type Deps struct{ DB *sql.DB }

// Start opens the database. Its notes table comes from .aicoded/migration.sql, the app's draft
// of schema changes.
func (d *Deps) Start(ctx context.Context) error {
	db, err := sqldb.Open(ctx)
	if err != nil {
		return err
	}
	d.DB = db
	return nil
}

// Notes returns every note, newest first.
func (d *Deps) Notes(ctx context.Context) ([]Note, error) {
	rows, err := d.DB.QueryContext(ctx, "SELECT id, owner, title, body FROM notes ORDER BY id DESC")
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

// Add stores a note written by owner.
func (d *Deps) Add(ctx context.Context, owner, title, body string) error {
	_, err := d.DB.ExecContext(ctx, "INSERT INTO notes (owner, title, body) VALUES (?, ?, ?)", owner, title, body)
	return err
}
