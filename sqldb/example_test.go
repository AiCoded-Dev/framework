package sqldb_test

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/sqldb"
)

var db *sql.DB // the database Open returned in OnStart

// Open the database once, in OnStart, and create the app's tables there. Keep the *sql.DB in the
// app's deps for every page.
func ExampleOpen() {
	onStart := func(ctx context.Context) error {
		var err error
		db, err = sqldb.Open(ctx)
		if err != nil {
			return err
		}
		_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS notes (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			owner VARCHAR(255) NOT NULL,
			title VARCHAR(200) NOT NULL,
			created DATETIME NOT NULL)`)
		return err
	}
	_ = onStart
}

// Pass every value with a ? placeholder, here to list the viewer's notes in a page's Data.
func ExampleOpen_query() {
	data := func(ctx context.Context) error {
		rows, err := db.QueryContext(ctx, "SELECT id, title FROM notes WHERE owner = ? ORDER BY id", auth.Viewer(ctx).Subject)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var title string
			if err := rows.Scan(&id, &title); err != nil {
				return err
			}
			fmt.Println(id, title)
		}
		return rows.Err()
	}
	_ = data
}

// Run statements that belong together in a transaction, here in a form's Process hook.
func ExampleOpen_transaction() {
	process := func(ctx context.Context) error {
		owner := auth.Viewer(ctx).Subject
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }() // does nothing after Commit
		_, err = tx.ExecContext(ctx, "INSERT INTO notes (owner, title, created) VALUES (?, ?, ?)", owner, "Quarterly review", time.Now())
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE owners SET notes = notes + 1 WHERE owner = ?", owner)
		if err != nil {
			return err
		}
		return tx.Commit()
	}
	_ = process
}
