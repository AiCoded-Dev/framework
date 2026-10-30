# E-MAIL-011: a preview's message over 1 MiB

On the platform, an app's preview sends no mail: the runner hands each message to the platform,
which keeps it for the app's owner to read in the console, and nothing reaches its recipients.
The platform keeps a message of up to 1 MiB, counted as the whole message with its attachments,
and refuses a larger one. The app's `mailer.Send` returns this error, and nothing was kept. The
preview keeps the newest 200 messages and drops the oldest.

**Fix:** send a smaller message; a preview keeps messages up to 1 MiB
