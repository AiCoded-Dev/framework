# E-CHK-006: go mod tidy failed

`aicoded init` runs `go mod tidy` to complete the new app's `go.mod` and `go.sum`, which downloads the modules the app needs. It failed. The message carries what it printed, most often a network error or a line of `go.mod` it cannot read. `aicoded init` then removes the new app's folder, so its fix line asks you to run `aicoded init <name>` again once the network works, not `go mod tidy`.

**Fix:** check the network, then run `aicoded init <name>` again.
