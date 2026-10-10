# E-GATE-022: a module has a known weakness the app does not call

This is a note: it does not stop the publish. govulncheck or OSV-Scanner, which the delivery pipeline runs over the modules the app uses, found a known weakness (a vulnerability) in a module at the version the app uses, but the app's code never calls the weak function. The app is not exposed today, but a later change could call it, so upgrade the module when you can, once a version of it fixes the weakness. The note is at the module's line in `go.mod`, and the message names the weakness, its aliases and score, the module, the version the app uses and the version that fixes it, when there is one. A weakness of the Go standard library is at the `go` line: the platform chooses the Go that apps build with, so tell your administrator, who updates Go once a release fixes it; the message names that release when there is one. The change record counts the note.

```text
require golang.org/x/text v0.3.7   // a weakness in a function of the module that the app never calls
```

**Fix:** the one for the case the message names:

- a version fixes it: upgrade the module when you can; the app does not call the weak code;
- no version fixes it: no version of the module fixes it yet; the app does not call the weak code;
- a Go release fixes it: tell your administrator that `<release>` fixes it: the platform chooses the Go that apps build with;
- no Go release fixes it: tell your administrator: no Go release fixes it yet, and the platform chooses the Go that apps build with.
