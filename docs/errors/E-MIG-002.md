# E-MIG-002: a statement that a schema change may not hold

A draft changes only the schema of the app's own tables. Its statements may add to it, so that the version of the app that runs now keeps working while the new one starts:

- `CREATE TABLE <name> (…)` with the table options `ENGINE=InnoDB`, `CHARSET`, `COLLATE`, `COMMENT`, `AUTO_INCREMENT` and `ROW_FORMAT`;
- `CREATE [UNIQUE|FULLTEXT|SPATIAL] INDEX <name> ON <table> (…)`;
- `ALTER TABLE <table>` with `ADD` clauses: a column that is NULL or has a `DEFAULT`, an index or key, or a `PRIMARY KEY`, `UNIQUE` or `FOREIGN KEY` constraint.

They may also be one of the destructive forms that E-MIG-003 lists, such as `DROP TABLE <table>` or `ALTER TABLE <table> DROP COLUMN <column>`, which need an approval.

Everything else is refused: statements that change data (`INSERT`, `UPDATE`, `DELETE`), `SET`, `USE`, `GRANT`, `LOCK`, `CREATE VIEW`, `TRIGGER`, `PROCEDURE`, `FUNCTION`, `EVENT`, `DATABASE` and `USER`, `CREATE TABLE … LIKE` or `… AS SELECT`, `IF NOT EXISTS`, partitions, every other form of `DROP`, `RENAME`, `TRUNCATE` and `ALTER TABLE`, such as `DROP DATABASE`, `DROP VIEW`, `ALTER TABLE … ENGINE` or `ALTER TABLE … ALGORITHM`, and a new column that is `NOT NULL` without a `DEFAULT`. So is what hides SQL from the checks or reaches past the app's own tables: executable comments `/*! … */` and hints `/*+ … */`, `DELIMITER`, a name with a database in front (`db.table`), strings in double quotes, backslashes in strings, variables, the table `aicoded_migrations` in any case, a name such as `modify` or `truncate` that reads as a keyword unless it is in backquotes, and text the checks cannot read, such as a string or a comment that does not end. A destructive statement in one of the allowed forms is the note E-MIG-003.

```sql
INSERT INTO statuses (name) VALUES ('open'); -- wrong: data belongs to the app's code
ALTER TABLE rooms ADD COLUMN floor INT NOT NULL; -- wrong: the running version cannot add rows
ALTER TABLE rooms ADD COLUMN floor INT NOT NULL DEFAULT 0; -- right
```

**Fix:** write only CREATE TABLE, CREATE INDEX and ALTER TABLE … ADD statements on the app's own tables, or the destructive forms of E-MIG-003, give a new NOT NULL column a DEFAULT, and change data from the app's code
