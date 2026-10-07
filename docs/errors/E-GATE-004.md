# E-GATE-004: the go line of go.mod is too new

The `go` line of `go.mod` asks for a newer Go than the one the delivery pipeline builds with, which the message names. The problem is at the line.

**Fix:** lower the `go` line to the version the message names, or an older one, with `go mod edit -go=<version>`.
