# E-MAIL-012: a received message the app cannot read

In your own cloud, the app's mailbox keeps the mail sent to its `email.from`, and the runner reads
each message the app asks for and hands it the sender, recipients, subject, text, HTML and
attachments. This message is larger than 10 MiB, or is not a message the runner can read: its
header or its parts are broken, it has more than 100 parts, or its parts nest deeper than 8. A
message that grows past 16 MiB once the runner converts its charsets to UTF-8 is answered the
same way. The message stays in the mailbox, and `mailer.List` shows it with its id and size;
a message whose head the runner can read, as one that is only too large, is listed with its sender
and subject too, and one whose head it cannot read without them. Nothing of the message is in the
runner's log.

**Fix:** move the message to trash or delete it with `mailer.Move` or `mailer.Delete`, and ask its sender to send it again, smaller.
