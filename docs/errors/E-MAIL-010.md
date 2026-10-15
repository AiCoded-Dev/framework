# E-MAIL-010: too many messages being sent

On the platform, the runner remembers the idempotency keys of the app's last 10 000 messages within
24 hours, so that a retry sends nothing again. While 10 000 of the app's messages are still being
sent, as when the mail server is slow to answer, the runner cannot take a message with a new key
without forgetting a key under way, so it refuses the message. Nothing was sent.

**Fix:** send the message again once the app's messages under way are sent, and send fewer messages at once
