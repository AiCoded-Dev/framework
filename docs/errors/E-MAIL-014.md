# E-MAIL-014: the company's mail relay cannot be used now

In your own cloud, the company can have the runner send every app's mail through its own mail
relay instead of Amazon SES. The runner signs in to the relay with a user and password that the
company keeps in AWS Secrets Manager, and reads them again at least once a minute. The runner
could not read them now: the secret is missing, the server is not allowed to read it, or it is not
the relay's sign-in. Or the relay refused the sign-in a moment ago, as when the password changed:
the runner then reads the secret again a few seconds later. Or the relay does not take AUTH PLAIN,
the only way the runner signs in, and answered it with 504: the runner remembers that for 5 seconds
before it tries the relay again. Nothing was sent, and the runner's log tells its operator what to
fix.

**Fix:** send the message again later, with the same idempotency key; if it keeps failing, ask the company's cloud administrator to check the mail relay's secret; the relay must offer AUTH PLAIN inside TLS.
