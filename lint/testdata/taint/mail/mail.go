// Package mail sends mail that carries outside data. A send whose message may carry data from
// the app's database, its files, its inbox or a secret ends in a comment "note:" with the note it
// makes; a send that carries none of them ends in "clean".
package mail

import (
	"context"
	"database/sql"
	"strconv"

	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/filestore"
	"aicoded.dev/framework/mailer"
	"aicoded.dev/framework/secrets"
	"aicoded.dev/framework/web/form"
)

// Database mails a name and a count read from the database.
func Database(ctx context.Context, db *sql.DB) error {
	var name string
	var n int
	if err := db.QueryRowContext(ctx, "SELECT name, COUNT(*) FROM users").Scan(&name, &n); err != nil {
		return err
	}
	_, err := mailer.Send(ctx, mailer.Message{IdempotencyKey: "db", Subject: "Hello " + name}) // note: the app's database reaches mail
	if err != nil {
		return err
	}
	_, err = mailer.Send(ctx, mailer.Message{IdempotencyKey: "count", Text: strconv.Itoa(n) + " users"}) // clean
	return err
}

// Files mails a stored file and its size.
func Files(ctx context.Context, store *filestore.Store) error {
	data, err := store.ReadFile("invoices/a.pdf")
	if err != nil {
		return err
	}
	_, err = mailer.Send(ctx, mailer.Message{IdempotencyKey: "file", Attachments: []mailer.Attachment{{Filename: "a.pdf", Data: data}}}) // note: its files reach mail
	if err != nil {
		return err
	}
	_, err = mailer.Send(ctx, mailer.Message{IdempotencyKey: "size", Text: strconv.Itoa(len(data)) + " bytes"}) // clean
	return err
}

// Inbox forwards a mail from the inbox, and mails its size.
func Inbox(ctx context.Context) error {
	m, err := mailer.Get(ctx, "m1")
	if err != nil {
		return err
	}
	_, err = mailer.Send(ctx, mailer.Message{IdempotencyKey: "forward", Text: m.Text}) // note: its inbox reaches mail
	if err != nil {
		return err
	}
	_, err = mailer.Send(ctx, mailer.Message{IdempotencyKey: "size", Text: strconv.FormatInt(m.Size, 10)}) // clean
	return err
}

// Secret mails a secret and its length.
func Secret(ctx context.Context) error {
	key, err := secrets.Get(ctx, "partner_api_key")
	if err != nil {
		return err
	}
	_, err = mailer.Send(ctx, mailer.Message{IdempotencyKey: "key", Text: key.Reveal()}) // note: a secret reaches mail
	if err != nil {
		return err
	}
	_, err = mailer.Send(ctx, mailer.Message{IdempotencyKey: "length", Text: strconv.Itoa(len(key.Reveal()))}) // clean
	return err
}

// Reply mails a form value to the viewer.
func Reply(ctx context.Context, f *form.Textarea) error {
	v := auth.Viewer(ctx)
	_, err := mailer.Send(ctx, mailer.Message{ // clean
		IdempotencyKey: "reply", To: []mailer.Address{{Name: v.Name, Address: v.Subject}}, Text: f.GetValue(),
	})
	return err
}

// Digest mails a name from the database and a secret through a function value and a helper.
func Digest(ctx context.Context, db *sql.DB) error {
	var name string
	if err := db.QueryRowContext(ctx, "SELECT name FROM users LIMIT 1").Scan(&name); err != nil {
		return err
	}
	key, err := secrets.Get(ctx, "partner_api_key")
	if err != nil {
		return err
	}
	send := mailer.Send
	_, err = send(ctx, mailer.Message{IdempotencyKey: "digest", Text: name + key.Reveal()}) // note: the app's database or a secret reaches mail
	if err != nil {
		return err
	}
	if err := notify(ctx, name); err != nil {
		return err
	}
	return notify(ctx, "constant")
}

// notify mails text.
func notify(ctx context.Context, text string) error {
	_, err := mailer.Send(ctx, mailer.Message{IdempotencyKey: text, Text: text}) // note: the app's database reaches mail
	return err
}
