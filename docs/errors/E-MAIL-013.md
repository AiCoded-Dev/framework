# E-MAIL-013: a mail folder too large to list

In your own cloud, the app's mailbox keeps the mail sent to its `email.from`, and `mailer.List`
shows a folder's newest 10 000 messages, newest first. To find them, the runner reads the names of
every message in the folder, and it reads at most 20 000. This folder holds more, so the runner
shows no page rather than one without the newest messages. `mailer.Get`, `mailer.Move` and
`mailer.Delete` still work on messages the app knows.

**Fix:** delete or archive messages as the app reads them, so that no folder grows past 20 000; to list the folder again now, have the customer remove its oldest messages from the mail bucket.
