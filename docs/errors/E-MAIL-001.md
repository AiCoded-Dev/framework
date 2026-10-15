# E-MAIL-001: missing or invalid idempotency key

Every message needs an idempotency key: a stable name of what the message is about. When a request is retried, the same key sends nothing a second time while the runner remembers the key: on the platform for 24 hours and until the runner restarts, and in `aicoded dev` until it restarts. A retry after that sends the message again. The key is 1 to 128 visible ASCII characters, without spaces.

**Fix:** set `IdempotencyKey`, such as `"invoice-42-reminder"`.
