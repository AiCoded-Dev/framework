# E-GATE-007: a framework version the platform does not build

The delivery pipeline builds apps with the framework versions it knows, which the message lists. `go.mod` requires another version of `aicoded.dev/framework`, or does not require the framework at all.

**Fix:** require one of the listed versions with `go get aicoded.dev/framework@<version>`, run `go mod tidy`, and commit.
