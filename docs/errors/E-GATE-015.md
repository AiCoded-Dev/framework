# E-GATE-015: an error is not checked

errcheck, which the delivery pipeline runs through golangci-lint, found a call whose error the code drops: the call's result is not used, or the call stands alone on its line. When such a call fails, the app goes on as if it had worked, so a write that did not happen looks done to the viewer. The message names the call, at its line. `aicoded check` does not run errcheck, so only a publish shows it.

```go
db.ExecContext(ctx, "UPDATE rooms SET clean = 1 WHERE id = ?", id) // wrong: the error is dropped

if _, err := db.ExecContext(ctx, "UPDATE rooms SET clean = 1 WHERE id = ?", id); err != nil { // right
	return err
}
```

**Fix:** handle the error the call returns, or return it.
