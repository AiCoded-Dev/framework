// Package mailer sends the app's mail and reads its mailbox. The app sends from its one address,
// email.from in the permission list, and only to the domains in email.to_domains; the runner
// enforces both:
//
//	email:
//	  from: ops@acme.example
//	  to_domains: [acme.example]
//
// Rules for [Send]:
//
//   - Every [Message] has an IdempotencyKey that names what the mail is about, such as
//     invoice-42-reminder. Sending the same key again sends nothing and returns the first
//     message's id, so a retried request never mails twice.
//   - The domain of every recipient, and of ReplyTo unless it is the app's own address, equals
//     one of email.to_domains (E-MAIL-003).
//   - Headers may set only In-Reply-To, References, List-Id, List-Unsubscribe,
//     List-Unsubscribe-Post, Auto-Submitted and Precedence.
//   - Send after the data the mail reports is committed, never before.
//
// The app's mailbox has the folders [Inbox], [Archive] and [Trash]. [List] reads a page of a
// folder, newest first, [Get] reads a message, [Move] moves it to another folder and [Delete]
// removes it. Without an email section in the permission list, every call fails with E-MAN-012.
// In aicoded dev, mail is caught and shown on the dev UI, never sent.
//
// Read more in the guide docs/guides/mail.md and the task docs/tasks/email-after-a-write.md, which
// aicoded explain and the MCP tool howto print as guides/mail and tasks/email-after-a-write.
package mailer
