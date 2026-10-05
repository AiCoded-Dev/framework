# E-CHK-003: go vet finding

`aicoded check` runs `go vet` on every package of the app. It found code that compiles but is very likely wrong, such as a `Printf` whose verbs do not match its arguments. The message starts with the name of the check that found it.

**Fix:** fix the code as the message says; go vet will run in the security checks too.
