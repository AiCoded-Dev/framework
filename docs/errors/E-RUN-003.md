# E-RUN-003: incomplete runner handshake

The runner answered the app's handshake without the app name, a CSRF key of at least 32
bytes, or valid Ed25519 viewer keys. The app refuses to start rather than serve without them.

**Fix:** update `aicoded` (or the platform runner) to a version that speaks the runner protocol
named in the error.
