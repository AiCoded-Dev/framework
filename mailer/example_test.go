package mailer_test

import (
	"context"
	"fmt"
	"time"

	"aicoded.dev/framework/mailer"
)

// Send a mail after the data it reports is committed, here in a form's Process hook. The
// idempotency key names what the mail is about, so a retried request sends it once.
func ExampleSend() {
	process := func(ctx context.Context) error {
		_, err := mailer.Send(ctx, mailer.Message{
			IdempotencyKey: "invoice-42-reminder",
			To:             []mailer.Address{{Name: "Accounts", Address: "accounts@acme.example"}},
			Subject:        "Invoice 42 is due tomorrow",
			Text:           "Invoice 42 is due tomorrow. Its details are in the app.",
		})
		return err
	}
	_ = process
}

// List reads a page of a folder of the app's mailbox, newest first, here in a page's Data.
func ExampleList() {
	data := func(ctx context.Context) error {
		page, total, err := mailer.List(ctx, mailer.Inbox, 0, 20)
		if err != nil {
			return err
		}
		fmt.Printf("%d of %d messages\n", len(page), total)
		for _, m := range page {
			fmt.Println(m.Date.Format(time.DateOnly), m.Subject)
		}
		return nil
	}
	_ = data
}
