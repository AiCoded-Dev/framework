# E-MAN-013: invalid modules entry

`modules:` in `aicoded.yaml` lists the third-party modules the app uses, by module path, one entry each, such as `golang.org/x/text`; their versions come from `go.mod`. An entry is not a valid module path, carries a version, is listed twice, or is the framework, `aicoded.dev/framework` or a module below it, which needs no declaration.

**Fix:** write each module path once, as go.mod requires it, with no version, and leave out the framework.
