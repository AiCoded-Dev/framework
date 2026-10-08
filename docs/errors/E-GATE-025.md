# E-GATE-025: a directive silences a check

This is a note: it does not stop the publish. A comment in the app's code holds a directive that silences a check: `//nolint` for golangci-lint, or `#nosec` or `//gosec:disable` for gosec. For now the security checks honour these directives, so the finding they silence does not stop the publish. Each directive is a note at its line instead, which names it, and the change record counts it, so security sees every finding the app silenced.

```go
db.ExecContext(ctx, "DELETE FROM drafts WHERE id = ?", id) //nolint:errcheck // wrong: a note

if _, err := db.ExecContext(ctx, "DELETE FROM drafts WHERE id = ?", id); err != nil { // right
	return err
}
```

See [publishing](../guides/publishing.md) for the checks the directives silence.

**Fix:** remove the directive and fix what the check reports.
