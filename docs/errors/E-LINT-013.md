# E-LINT-013: schema change in app code

App code reads and writes rows; it never changes the schema. The SQL that an app passes to `Query`, `QueryContext`, `QueryRow`, `QueryRowContext`, `Exec`, `ExecContext`, `Prepare` or `PrepareContext` of `*sql.DB`, `*sql.Tx` or `*sql.Conn` may not start with `CREATE`, `ALTER`, `DROP`, `RENAME` or `TRUNCATE`, in any case and after any comment or parenthesis. On the platform the app's database user may only read and write rows, so such a statement would fail there anyway. Each schema change goes in the app's draft, `.aicoded/migration.sql`, instead: `aicoded check` checks it, `aicoded publish` sends it with the commit, and the platform applies it to the app's database, as the database's owner, when the release rolls out. A draft may only add to the schema (E-MIG-002, E-MIG-003), and it is never committed to git (E-MIG-001).

```go
db.ExecContext(ctx, "CREATE TABLE notes (id BIGINT AUTO_INCREMENT PRIMARY KEY, body TEXT NOT NULL)") // wrong
```

```sql
-- .aicoded/migration.sql: right
CREATE TABLE notes (id BIGINT AUTO_INCREMENT PRIMARY KEY, body TEXT NOT NULL);
```

**Fix:** write it in .aicoded/migration.sql; the platform applies it when the release rolls out
