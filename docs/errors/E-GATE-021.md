# E-GATE-021: the app calls code with a known weakness

This is a note: it does not stop the publish. govulncheck, which the delivery pipeline runs with OSV-Scanner over the modules the app uses, found that the app's code calls a function with a known weakness (a vulnerability) that is not critical, or that is in the Go standard library, whatever its score. A weakness is critical when its score, the highest CVSS score of the weakness and its aliases in the OSV database, is 9.0 or more; a lower score, or no score yet, is not critical. A critical weakness outside the Go standard library is the problem E-GATE-020 instead. The app still runs the weak code, so upgrade the module. The note is at the module's line in `go.mod`, and the message names the weakness, its aliases and score, the module, the version the app uses and the version that fixes it, and the calls from the app's code to the weak function. A weakness of the Go standard library is at the `go` line: a newer Go fixes it, and the platform chooses its Go, so tell your administrator. The change record counts the note.

```sh
go get golang.org/x/text@v0.31.0   # the module and fixed version the message names
go mod tidy
```

**Fix:** upgrade the module to the fixed version the message names.
