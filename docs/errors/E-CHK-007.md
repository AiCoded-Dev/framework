# E-CHK-007: aicoded is older than the go command

`aicoded check` lints every app by reading its Go code, and the standard library it uses, with the Go that `aicoded` itself was built with. The go command that builds the app, the one on `PATH` or the one `GOTOOLCHAIN` selects, as `go env GOVERSION` prints it in the app's folder, is a newer Go than that, so lint cannot read the code and no app passes the check. A newer patch release of the same Go, such as go1.25.14 for an `aicoded` built with go1.25.8, is fine. The message names both versions.

**Fix:** install aicoded again with the Go you build with, or set GOTOOLCHAIN to the Go aicoded was built with; the fix in the message names both versions.
