# E-GATE-003: toolchain line in go.mod

`go.mod` has a `toolchain` line, which asks the go command for a Go toolchain of the app's choice. The delivery pipeline builds every app with the Go it pins itself, and refuses the line. The problem is at the line.

**Fix:** remove the `toolchain` line, for example with `go mod edit -toolchain=none`.
