# E-MIG-003: a destructive schema change

This is a note: it never stops `aicoded check` or a publish. A statement of the draft removes, renames or changes what the schema of the app's own tables has:

- `DROP TABLE [IF EXISTS] <table>[, <table>…]`, `DROP INDEX <name> ON <table>`, `RENAME TABLE <a> TO <b>[, …]` and `TRUNCATE [TABLE] <table>`;
- `ALTER TABLE <table>` with one of these clauses, alone or among `ADD` clauses: `DROP [COLUMN] <column>`, `DROP INDEX` or `KEY <name>`, `DROP FOREIGN KEY <name>`, `DROP PRIMARY KEY`, `DROP CHECK` or `CONSTRAINT <name>`, `MODIFY`, `CHANGE`, `RENAME TO` or `AS <table>`, `RENAME COLUMN`, `INDEX` or `KEY <a> TO <b>`, and `ALTER [COLUMN] <column>` with `SET DEFAULT`, `DROP DEFAULT`, `SET VISIBLE` or `SET INVISIBLE`.

Any other form of `DROP`, `RENAME` or `ALTER`, such as `DROP DATABASE`, `DROP VIEW`, `DROP USER`, a name with a database in front or the table `aicoded_migrations`, is E-MIG-002.

The version of the app that runs now still uses what the statement removes or changes, and the data it removes cannot come back. So a publish whose draft holds one *Needs approval*: it goes live only once an approver approved it. When it goes live, the platform stops the app, applies the steps and starts the new version; the app is unavailable meanwhile, usually for seconds. When a step fails, the version that ran starts again. Once a destructive step ran, a rollback to a release before it is refused (E-MIG-007).

`aicoded check` shows the note at the statement's line, and `aicoded dev` applies the statement to the dev database, whose data is test data.

```sql
ALTER TABLE rooms ADD COLUMN level VARCHAR(20) NULL; -- expand-only: goes live without an approval; the code stops using floor
ALTER TABLE rooms DROP COLUMN floor; -- destructive: needs an approval, and the app stops briefly
```

Where you can, make the change expand-only instead: add a new column or table, move the code to it and stop using the old one. A release of such a change needs no approval and the app keeps serving.

**Fix:** this release needs an approval, and the app stops briefly while the change is applied
