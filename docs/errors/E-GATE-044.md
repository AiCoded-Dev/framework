# E-GATE-044: a destructive schema change

This is a note: it does not stop the publish. It is a reason its release *Needs approval*, and `aicoded publish` lists it under `Needs approval:`. The schema changes of the release hold a destructive one (E-MIG-003), such as a dropped column or a renamed table, that production has not run. Whatever the app type allows, such a change always needs an approver: the version that runs still uses what it removes, the data it removes cannot come back, and the app stops briefly while it is applied. The note names the step as `<publish>#<index>` and never quotes its statement, which may hold data; the change record holds the statement as the security checks scrubbed it of secrets. See [app types](../guides/app-types.md).

```sql
ALTER TABLE rooms DROP COLUMN floor;
```

**Fix:** an approver sees the destructive change and the short stop on the approval card; to stay Pre-approved, make the change expand-only: add a column or table and stop using the old one
