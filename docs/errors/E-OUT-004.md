# E-OUT-004: request body too large

The body of a request the app sent with `outbound.Client()` is over 10 MiB. Nothing was sent.

**Fix:** send at most 10 MiB; keep large files in a file store
