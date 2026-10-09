# E-CHK-005: no go command

`aicoded check`, `aicoded dev` and `aicoded init` build and test apps with the Go toolchain. They did not find the `go` command on `PATH`.

**Fix:** install Go 1.26 or newer and put go on PATH.
