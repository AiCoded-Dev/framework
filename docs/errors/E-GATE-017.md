# E-GATE-017: SQL rows or a statement are not closed

sqlclosecheck, which the delivery pipeline runs through golangci-lint, found `*sql.Rows` or a `*sql.Stmt` that the code never closes. Each one holds a connection to the database until it is closed, so the app soon has none left and its pages stop loading. The problem is at the line of the query or statement. `aicoded check` does not run sqlclosecheck, so only a publish shows it. See [the SQL database](../guides/sql-database.md).

```go
rows, err := db.QueryContext(ctx, "SELECT id, name FROM rooms") // wrong: rows are never closed
if err != nil {
	return nil, err
}

rows, err := db.QueryContext(ctx, "SELECT id, name FROM rooms") // right
if err != nil {
	return nil, err
}
defer rows.Close()
```

**Fix:** close the rows or statement with defer rows.Close() right after the query.
