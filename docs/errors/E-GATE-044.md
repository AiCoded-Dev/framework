# E-GATE-044: a destructive schema change

This is a note: it does not stop the publish. It is a reason the release *Needs approval*: the publish passed, but it goes live only once an approver approved it, and `aicoded publish` and `aicoded status` list it under `Needs approval:`. Until the console exists, the platform's operator releases such a publish when your organisation asks. The schema changes of the release hold a destructive one (E-MIG-003), such as a dropped column or a renamed table, that production has not run. Whatever the app type allows, such a change always needs an approver: the version that runs still uses what it removes, the data it removes cannot come back, and the app stops briefly while it is applied. See [app types](../guides/app-types.md).

```sql
ALTER TABLE rooms DROP COLUMN floor;
```

**Fix:** an approver sees the destructive change and the short stop on the approval card; to stay Pre-approved, make the change expand-only: add a column or table and stop using the old one
