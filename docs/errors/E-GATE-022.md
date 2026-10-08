# E-GATE-022: a module has a known weakness the app does not call

This is a note: it does not stop the publish. govulncheck or OSV-Scanner, which the delivery pipeline runs over the modules the app uses, found a known weakness (a vulnerability) in a module at the version the app uses, but the app's code never calls the weak function. The app is not exposed today, but a later change could call it, so upgrade the module when you can. The note is at the module's line in `go.mod`, and the message names the weakness, its aliases and score, the module, the version the app uses and the version that fixes it. The change record counts the note.

```text
require golang.org/x/text v0.3.7   // a weakness in a function of the module that the app never calls
```

**Fix:** upgrade the module when you can; the app does not call the weak code.
