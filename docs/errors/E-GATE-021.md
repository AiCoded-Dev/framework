# E-GATE-021: the app calls code with a known weakness

This is a note: it does not stop the publish. govulncheck, which the delivery pipeline runs with OSV-Scanner over the modules the app uses, found that the app's code calls a function with a known weakness (a vulnerability) whose score is below 9.0, or that has no score yet. The score is the highest CVSS score of the weakness and its aliases in the OSV database; at 9.0 or more the weakness would be the problem E-GATE-020 instead. The app still runs the weak code, so upgrade the module. The note is at the module's line in `go.mod`, and the message names the weakness, its aliases and score, the module, the version the app uses and the version that fixes it, and the calls from the app's code to the weak function. The change record counts the note.

```sh
go get golang.org/x/text@v0.31.0   # the module and fixed version the message names
go mod tidy
```

**Fix:** upgrade the module to the fixed version the message names.
