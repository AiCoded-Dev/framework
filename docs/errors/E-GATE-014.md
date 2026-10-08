# E-GATE-014: gosec found a security weakness

gosec, which the delivery pipeline runs through golangci-lint, found code with a known security weakness. The message starts with the rule's id and says what is wrong at the line: such as `G404` for a random value that can be guessed, `G115` for an integer conversion that may overflow, or `G101` for a password written in the code. An attacker can use such a weakness even when the app works as intended. A `#nosec` comment silences gosec for now, but each one is a note (E-GATE-025) that security sees in the change record, so fix the code instead. `aicoded check` does not run gosec, so only a publish shows it.

```go
// with math/rand/v2
code := fmt.Sprintf("%06d", rand.IntN(1_000_000)) // wrong: G404, the code can be guessed

// with crypto/rand
code := rand.Text() // right: for anything that must stay secret
```

**Fix:** fix the code at that line as the message says: do not suppress it; commit, and publish again.
