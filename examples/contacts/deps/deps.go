// Package deps holds the contacts database. Start opens it before the app serves calls, and the
// functions in rpc/ read it with Contacts.
package deps

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"aicoded.dev/framework/sqldb"
)

// ErrNoPerson is the error of a login with no contacts.
var ErrNoPerson = errors.New("no such person")

// db is the app's database. Start sets it before the runner sends the app any call.
var db *sql.DB

// Start opens the app's database and prepares the contacts table.
func Start(ctx context.Context) error {
	conn, err := sqldb.Open(ctx)
	if err != nil {
		return err
	}
	if err := prepare(ctx, conn); err != nil {
		return err
	}
	db = conn
	return nil
}

// Contacts returns the phone numbers and email addresses of login, in the order they were added,
// or ErrNoPerson.
func Contacts(ctx context.Context, login string) (phones, emails []string, err error) {
	rows, err := db.QueryContext(ctx, "SELECT kind, value FROM contacts WHERE login = ? ORDER BY id", login)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind, value string
		if err := rows.Scan(&kind, &value); err != nil {
			return nil, nil, err
		}
		switch kind {
		case "phone":
			phones = append(phones, value)
		case "email":
			emails = append(emails, value)
		default:
			return nil, nil, fmt.Errorf("unknown contact kind %q", kind)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if phones == nil && emails == nil {
		return nil, nil, ErrNoPerson
	}
	return phones, emails, nil
}

// prepare fills the contacts table, which .aicoded/migration.sql creates, with seed when it is
// empty.
func prepare(ctx context.Context, conn *sql.DB) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var n int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM contacts").Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	for _, c := range seed {
		if _, err := tx.ExecContext(ctx, "INSERT INTO contacts (login, kind, value) VALUES (?, ?, ?)", c.login, c.kind, c.value); err != nil {
			return err
		}
	}
	return tx.Commit()
}
