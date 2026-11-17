# E-MAIL-010: too many messages being sent or read

On the platform, the runner remembers the idempotency keys of the app's last 10 000 messages within
24 hours, so that a retry sends nothing again. While 10 000 of the app's messages are still being
sent, as when the mail server is slow to answer, the runner cannot take a message with a new key
without forgetting a key under way, so it refuses the message. Nothing was sent.

In your own cloud, the runner also reads at most two messages of the app's mailbox at a time for
`mailer.Get`. A read that waits for one of them until its call's deadline passes is refused the
same way. Nothing in the mailbox changed.

**Fix:** send or read the message again once the app's messages under way are sent or read, and send or read fewer messages at once
