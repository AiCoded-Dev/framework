// Package deps holds what the pages share: the database and the docs store.
package deps

import (
	"context"
	"database/sql"

	"aicoded.dev/framework/filestore"
	"aicoded.dev/framework/sqldb"
)

// Item is one row of the items table.
type Item struct {
	ID    int64
	Title string
}

// Deps is filled by Start before the app serves.
type Deps struct {
	DB   *sql.DB
	Docs *filestore.Store
}

// Start opens the database and the docs store and creates the items table.
func (d *Deps) Start(ctx context.Context) error {
	db, err := sqldb.Open(ctx)
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS items (id BIGINT AUTO_INCREMENT PRIMARY KEY, title VARCHAR(200) NOT NULL)"); err != nil {
		return err
	}
	docs, err := filestore.Open(ctx, "docs")
	if err != nil {
		return err
	}
	d.DB, d.Docs = db, docs
	return nil
}
