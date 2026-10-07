# E-GATE-008: a module does not match its checksum

The delivery pipeline downloads every module the app uses and checks each against the Go checksum database. A line of `go.sum` does not match what it downloaded: the line was edited, or the module's files changed after the checksum database recorded them. The message names the module.

**Fix:** delete the module's lines from `go.sum`, run `go mod tidy` with the public checksum database, and commit; if the lines come back the same, tell your administrator.
