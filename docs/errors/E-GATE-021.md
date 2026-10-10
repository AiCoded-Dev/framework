# E-GATE-021: the app calls code with a known weakness

This is a note: it does not stop the publish. govulncheck, which the delivery pipeline runs with OSV-Scanner over the modules the app uses, found that the app's code calls a function with a known weakness (a vulnerability) that is not critical, or that is in the Go standard library, whatever its score. A weakness is critical when its score, the highest CVSS score of the weakness and its aliases in the OSV database, is 9.0 or more; a lower score, or no score yet, is not critical. A critical weakness outside the Go standard library is the problem E-GATE-020 instead. The app still runs the weak code, so upgrade the module, or, when no version of it fixes the weakness yet, stop calling the weak function or use another module. The note is at the module's line in `go.mod`, and the message names the weakness, its aliases and score, the module, the version the app uses and the version that fixes it, when there is one, and the calls from the app's code to the weak function. A weakness of the Go standard library is at the `go` line: the platform chooses the Go that apps build with, so tell your administrator, who updates Go once a release fixes it; the message names that release when there is one. The change record counts the note.

```sh
go get golang.org/x/text@v0.31.0   # the module and fixed version the message names
go mod tidy
```

**Fix:** the one for the case the message names:

- a version fixes it: upgrade the module to the fixed version the message names;
- no version fixes it: no version of the module fixes it yet: stop calling the weak function, or use another module;
- a Go release fixes it: tell your administrator that `<release>` fixes it: the platform chooses the Go that apps build with;
- no Go release fixes it: tell your administrator: no Go release fixes it yet, and the platform chooses the Go that apps build with.
