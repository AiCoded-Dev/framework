# E-MAIL-001: missing or invalid idempotency key

Every message needs an idempotency key: a stable name of what the message is about. When a request is retried, the same key sends nothing a second time, so people never get the same mail twice. The key is 1 to 128 visible ASCII characters, without spaces.

**Fix:** set `IdempotencyKey`, such as `"invoice-42-reminder"`.
