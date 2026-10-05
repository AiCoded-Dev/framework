# E-RUN-002: runner protocol mismatch

The app was built with a framework that speaks a different app–runner protocol version than
the runner. The runner refuses the handshake rather than guess.

**Fix:** build the app with the framework version that matches `aicoded` (or the platform
runner): update the `aicoded.dev/framework` requirement in the app's go.mod, or update
`aicoded`.
