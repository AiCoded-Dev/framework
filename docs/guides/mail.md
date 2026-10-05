# Mail

`mailer.Send` sends a message from the app's one address to the domains its permission list
allows, and `mailer.List` and `mailer.Get` read the app's mailbox. The runner checks every message
against the mail rules, and every message carries an idempotency key, so a retried request never
mails twice.

## Declaring mail

```yaml
email:
  from: people@acme.example      # the one address the app sends from
  to_domains: [acme.example]     # the only domains it may send to
```

Without an `email` section, every mail call fails with E-MAN-012.

## Sending

```go
_, err := mailer.Send(ctx, mailer.Message{
	IdempotencyKey: "user-" + u.Login,
	To:             []mailer.Address{{Address: p.d.TeamAddress}},
	Subject:        "User " + u.Login + " added",
	Text:           fmt.Sprintf("%s (%s) joined the team %s.\n", u.Name, u.Login, u.Team),
})
```

`Send` returns the message's id.

- The sender is always `email.from`. `FromName` sets only the name shown with it.
- The domain of every recipient in `To`, `Cc` and `Bcc` must equal one in `email.to_domains`
  exactly, so a subdomain is not enough, and so must the domain of `ReplyTo`, unless it is the
  app's own address (E-MAIL-003).
- Addresses are plain ASCII addresses such as `name@acme.example`, with the person's name in
  `Name`, never inside the address (E-MAIL-002). A message needs at least one recipient.
- A message has `Text`, `HTML` or both (E-MAIL-006), and may carry `Attachments`, each with a
  `Filename`, a `ContentType` and its `Data`.
- `Headers` may set only `In-Reply-To`, `References`, `List-Id`, `List-Unsubscribe`,
  `List-Unsubscribe-Post`, `Auto-Submitted` and `Precedence`; the runner writes the envelope
  headers, such as `From`, `To` and `Subject`, from the message's fields. A line break or other
  control character in a header field, a name or an attachment's name is refused, so no one can
  add hidden recipients (E-MAIL-004).
- One message goes to at most 50 recipients, carries at most 20 attachments and is at most
  10 MiB (E-MAIL-005). Save large files in a file store instead.

## The idempotency key

`IdempotencyKey` is required: a stable name of what the message is about, 1 to 128 visible ASCII
characters without spaces, such as `invoice-42-reminder` (E-MAIL-001). Sending the same key again
sends nothing and returns the first message's id.

- Name the event, not the attempt: `user-alice` for "alice was added", never a random value or
  the time.
- When the key must come from what a viewer wrote, hash it: the contact form of the people
  example uses the first 32 hex digits of the SHA-256 of the viewer, the topic and the message,
  so the same message sent twice is mailed once and the key holds no personal data.

## After a write

Send the mail after the data it tells about is committed. `ProcessAdd` in the people example
commits the new user and its photos first, then mails the team. If the mail fails, the user stays
added, and the key `user-<login>` makes sure the team hears of each user once, however often it is
sent.

## The mailbox

The app has one mailbox, the `email.from` address, with the folders `mailer.Inbox`,
`mailer.Archive` and `mailer.Trash` (E-MAIL-007).

| Call | Does |
|---|---|
| `mailer.List(ctx, folder, offset, limit)` | a page of a folder, newest first, and the number of messages in it; at most 100, and a limit of 0 means 50 |
| `mailer.Get(ctx, id)` | a whole message: the summary, `Cc`, `ReplyTo`, `Text`, `HTML`, `Attachments` and `Headers` |
| `mailer.Move(ctx, id, folder)` | moves a message to another folder |
| `mailer.Delete(ctx, id)` | removes a message |

An unknown id is `mailer.ErrNotFound`.

```go
mail, total, err := mailer.List(ctx, mailer.Inbox, 0, 50)
```

## Traces

Every call is a span, `mailer.send`, `mailer.list`, `mailer.get`, `mailer.move` or
`mailer.delete`. Outside `aicoded dev`, a failed call records only its status and error code,
such as `invalid_argument E-MAIL-003`, since the runner's message may name a recipient's domain.
See [telemetry](telemetry.md).

## In aicoded dev

Mail is caught instead of sent. It is still checked against the mail rules and the permission
list, and kept in memory while `aicoded dev` runs: the last 1000 messages of each app, also
across restarts of the app.

- The Mail page of the dev UI shows what each app sent and has in its mailbox. A message's HTML
  part shows in a sandboxed frame that runs no script and loads nothing from the network.
- "Simulate inbound mail" on the dev UI, or the `mail_receive` tool of `aicoded mcp`, puts a
  message in an app's inbox.
- The `mail_list` and `mail_get` tools let your AI assistant read the caught mail.

See [aicoded dev](dev.md).

## See also

- [The permission list](permission-list.md): the `email` section.
- [people/pages/users/add/dataprovider.go](../../examples/people/pages/users/add/dataprovider.go):
  mail after a write.
- [people/pages/contact/dataprovider.go](../../examples/people/pages/contact/dataprovider.go):
  a key made from what a viewer wrote.
- [Send mail after a write](../tasks/email-after-a-write.md): commit, then mail with an
  idempotency key.
