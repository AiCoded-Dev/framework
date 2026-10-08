# E-GATE-013: staticcheck found a bug

staticcheck, which the delivery pipeline runs through golangci-lint, found code that is wrong or does nothing, such as a value that is overwritten before it is read, a condition that is always true, or a call whose result is dropped. Such code usually hides a bug, like an error that is never checked. The message starts with the rule's id, such as `SA4006`, and says what is wrong at the line. `aicoded check` does not run staticcheck, so only a publish shows it.

```go
err := save(ctx, room)
err = notify(ctx, room) // wrong: SA4006, the error of save is never read
if err != nil {
	return err
}

if err := save(ctx, room); err != nil { // right: each error is checked
	return err
}
if err := notify(ctx, room); err != nil {
	return err
}
```

**Fix:** fix the code at that line as the message says, run aicoded check, commit, and publish again.
