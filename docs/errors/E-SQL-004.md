# E-SQL-004: too many database connections

One app may have at most 32 database connections open at once through the runner. The `*sql.DB` that `sqldb.Open` returns keeps at most 16, so an app reaches the limit only with a second `*sql.DB` or a raised pool limit. Within one pool, rows that are never read to the end or never closed make later queries wait for a free connection instead.

**Fix:** close every `*sql.Rows` and statement, share one `*sql.DB` from `sqldb.Open`, and do not raise its pool limit.
