# E-GATE-020: the app calls code with a critical known weakness

govulncheck, which the delivery pipeline runs with OSV-Scanner over the modules the app uses, found that the app's code calls a function with a known weakness (a vulnerability), and the weakness is critical. Its score is the highest CVSS score of the weakness and its aliases, such as its CVE and GHSA ids, in the OSV database; a score of 9.0 or more stops the publish. A lower score, or none, is the note E-GATE-021, and a weakness the app does not call is the note E-GATE-022. The problem is at the module's line in `go.mod`, and the message names the weakness, its aliases and score, the module, the version the app uses and the version that fixes it, and the calls from the app's code to the weak function. A weakness of the Go standard library never stops the publish, whatever its score: it is the note E-GATE-021 or E-GATE-022. The delivery pipeline reads its own copy of the vulnerability databases, so nothing in the repository, such as an `osv-scanner.toml`, hides a weakness.

```sh
go get golang.org/x/text@v0.31.0   # the module and fixed version the message names
go mod tidy
aicoded check
```

**Fix:** upgrade the module to the fixed version the message names: `go get <module>@<version>`, then `go mod tidy`.
