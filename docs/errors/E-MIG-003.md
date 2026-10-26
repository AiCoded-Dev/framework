# E-MIG-003: a destructive schema change

A statement of the draft removes, renames or changes what the schema has: `DROP`, `RENAME`, `TRUNCATE`, or `ALTER TABLE` with a `DROP`, `MODIFY`, `CHANGE`, `RENAME` or `ALTER` clause. The version of the app that runs now still uses what the statement removes or changes, and its data would be lost. Such a change needs an approval, which the platform does not offer yet, so it is refused.

```sql
ALTER TABLE rooms DROP COLUMN floor; -- wrong
ALTER TABLE rooms ADD COLUMN level VARCHAR(20) NULL; -- right: a new column; the code stops using floor
```

**Fix:** make the change expand-only: add a new column or table and stop using the old one; destructive changes need an approval, which comes later
