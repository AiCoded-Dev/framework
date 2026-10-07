# E-GATE-005: go.mod or go.sum is not tidy

The delivery pipeline reads the app's module graph from `go.mod` and `go.sum` alone, and fetches modules only from its own mirror. It could not: `go.mod` is not tidy, `go.sum` lacks a line for a module the app uses, or the go command failed on them, as the message quotes.

**Fix:** run `go mod tidy` in the app's folder, commit `go.mod` and `go.sum`, then publish again.
