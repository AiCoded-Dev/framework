# E-GEN-037: not an app directory

aicoded found no `go.mod` in the directory it was run in, or the `go.mod` has no `module` line.
An app is a Go module: `go.mod` names the module, and `pages/` and the optional `deps/` sit
next to it.

**Fix:** run aicoded in the app's directory, next to go.mod
