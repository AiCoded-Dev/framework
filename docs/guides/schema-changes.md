# Schema changes: the draft and the app's migrations

An app's tables come from its schema changes, never from its code: write the next change in the
draft `.aicoded/migration.sql`, and `aicoded publish` sends it with the commit.

## The draft

The draft is `.aicoded/migration.sql` in the app's folder: the SQL statements of the next schema
change, each ended by `;`, with comments where they help. room-maintenance creates its two
tables in its draft:

<!-- code: examples/room-maintenance/.aicoded/migration.sql -->
```sql
CREATE TABLE rooms (
	number SMALLINT UNSIGNED PRIMARY KEY,
	floor TINYINT UNSIGNED NOT NULL
);

-- An author is the viewer's Subject, of at most 512 bytes, and compares exactly, with no case
-- folding and no trailing-space padding, so "Alice" and "alice " are not "alice".
CREATE TABLE tickets (
	id BIGINT AUTO_INCREMENT PRIMARY KEY,
	room SMALLINT UNSIGNED NOT NULL,
	title VARCHAR(100) NOT NULL,
	details TEXT NOT NULL,
	status ENUM('open', 'in_progress', 'done') NOT NULL,
	author VARCHAR(512) COLLATE utf8mb4_0900_bin NOT NULL,
	created DATETIME NOT NULL,
	updated DATETIME NOT NULL,
	KEY (author),
	FOREIGN KEY (room) REFERENCES rooms (number)
);
```

The examples commit their drafts, since they live in the framework's repository and are never
published. An app of yours never commits its draft. The rules:

- The draft stays out of git. A new app's `.gitignore` lists it, and `aicoded publish` refuses a
  commit, or an index, that holds it (E-MIG-001): run `git rm --cached .aicoded/migration.sql`.
- It holds at most 64 KiB of UTF-8 text and 100 statements, and at least one statement. Split a
  larger change over two publishes (E-MIG-001).
- It is a plain file inside the app's folder, not a folder or a link out of it (E-MIG-001).
- The app's permission list declares `sqldb` (E-MIG-006).
- It holds only the statements below (E-MIG-002, E-MIG-003).

`aicoded check`, `aicoded dev` and the delivery pipeline check the draft with the same rules, so
a draft that passes `aicoded check` passes the security check *Safe data changes*. Each problem
names its line, as `.aicoded/migration.sql:3`.

Data never goes in the draft. Fill a table from the app's code, such as in `OnStart` when the
table is empty, as `fill` in room-maintenance does.

## The statements a draft may hold

A draft only adds to the schema of the app's own database:

- `CREATE TABLE <name> (…)`, with the table options `ENGINE=InnoDB`, `[DEFAULT] CHARACTER SET`
  or `CHARSET`, `COLLATE`, `COMMENT`, `AUTO_INCREMENT` and `ROW_FORMAT` only. Each step runs
  once, so leave out `IF NOT EXISTS`.
- `CREATE [UNIQUE|FULLTEXT|SPATIAL] INDEX <name> ON <table> (…)`, with nothing after the
  columns.
- `ALTER TABLE <table>` with `ADD` clauses only, separated by commas:
  - `ADD [COLUMN] <column> <type> …`: a new column that is `NOT NULL` or a `PRIMARY KEY` needs a
    `DEFAULT`, so that the running version can still add rows;
  - `ADD INDEX`, `ADD KEY`, `ADD UNIQUE`, `ADD FULLTEXT INDEX` and `ADD SPATIAL INDEX`;
  - `ADD PRIMARY KEY`, `ADD FOREIGN KEY` and `ADD CONSTRAINT [<name>]` with a `PRIMARY KEY`,
    `UNIQUE` or `FOREIGN KEY`.

Everything else is refused with E-MIG-002, and the fix says what to write instead:

- statements that change data, such as `INSERT`, `UPDATE` and `DELETE`, and every other kind of
  statement, such as `SET`, `USE`, `GRANT`, `CREATE VIEW` or `CREATE TRIGGER`;
- `CREATE TABLE … LIKE`, `… AS SELECT`, `TEMPORARY`, partitions, `ADD CHECK` and `ADD (…)`;
- what hides SQL from the checks or reaches past the app's own tables: `/*! … */` and `/*+ … */`,
  `DELIMITER`, a name with a database in front, such as `db.table`, strings in double quotes,
  backslashes in strings, variables such as `@x`, and subqueries;
- the table `aicoded_migrations`, in any letter case;
- a column named like a keyword, such as `modify`, unless it is in backquotes.

A statement that removes, renames or changes what is there is E-MIG-003: `DROP`, `RENAME`,
`TRUNCATE`, and `ALTER TABLE` with a `DROP`, `MODIFY`, `CHANGE`, `RENAME`, `TRUNCATE` or `ALTER`
clause.

```sql
ALTER TABLE tickets ADD COLUMN priority TINYINT UNSIGNED NOT NULL DEFAULT 2; -- right
ALTER TABLE tickets ADD COLUMN priority TINYINT UNSIGNED NOT NULL; -- wrong: E-MIG-002, no DEFAULT
ALTER TABLE tickets DROP COLUMN details; -- wrong: E-MIG-003
```

## Why destructive changes wait

A release starts the new version while the old one still serves, and switches over once the new
one is ready; a rollback starts an older version again. Both versions run on the same database,
so its schema must suit both: a dropped or renamed column breaks the version that still uses it,
and no rollback brings dropped data back. So a draft only adds. To replace a column, add a new
one, move the code to it, and stop using the old one. Destructive changes need an approval,
which comes later.

## On your computer

`aicoded dev` applies, before it starts the app, the migrations the app published, which it
fetches from the platform with your sign-in and caches in `.aicoded/migrations.json`, then the
draft. When the draft changes in a way other than growing, it makes the dev database again:
its data is test data. A refused draft or a failed step (E-MIG-005) stops the app.
[aicoded dev](dev.md) tells the details, under Migrations.

`aicoded check` makes a test database for each app that declares `sqldb`, when `dev.yaml` names
`mysql:`, with the same steps. The app's tests reach it as the app's data user, through
`sqltest.Open(t)` from `aicoded.dev/framework/sqldb/sqltest`, which skips the test elsewhere.
Tests that share the database start from a known state. room-maintenance empties its tables:

<!-- code: examples/room-maintenance/deps/deps_test.go testDB -->
```go
// testDB returns the test database that aicoded check makes with the tables of
// .aicoded/migration.sql, emptied, or skips the test outside aicoded check.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db := sqltest.Open(t)
	_, err := db.ExecContext(t.Context(), "DELETE FROM tickets")
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), "DELETE FROM rooms")
	require.NoError(t, err)
	return db
}
```

Like the app on the platform, the app in `aicoded dev` and its tests get a user that reads and
writes rows only: a `CREATE TABLE` from the app gets MySQL's access-denied error, and
`aicoded check` refuses one in app code (E-LINT-013).

## At publish

`aicoded publish`, or the `publish` tool, sends the draft with the commit. The delivery pipeline
checks it (*Safe data changes*). A publish that passes adds the draft's statements, as steps, to
the app's migrations: those of the publish it builds on, then these. A publish without a draft
keeps the migrations of the one before it, so a change that is not released yet is never lost.

Once the publish passed, `aicoded publish` deletes the draft and refreshes
`.aicoded/migrations.json`; after `--no-wait` or the `publish` tool, `aicoded status`, the
`release_status` tool or the next publish does it. A publish that failed leaves the draft as it
was, to fix and publish again. A draft you changed after it was sent stays. While the publish
that carried the draft has not ended, a new publish that still carries a draft is refused
(E-MIG-001).

## At release

When a release rolls out, the platform applies to the app's database, as the database's owner,
the steps the database has not run yet, in order, before it starts the new version. The app
itself only ever gets a user that reads and writes rows. The database keeps what ran in its
journal, the table `aicoded_migrations`, which app code may not read or write (E-LINT-013).

- A step that fails, waits more than 30 seconds for a table another session uses, or runs for
  more than 5 minutes stops the release (E-MIG-005). The version that runs keeps serving, and the
  steps before it stay applied. The message names the step and MySQL's error number, with a
  fixed short text such as `MySQL error 1062 (duplicate key)`, never MySQL's own message, which
  can quote the app's data.
- When MySQL refused the statement on production data, such as a unique key over rows that
  repeat a value, the schema changes after the steps that ran are rejected: no publish that holds
  them is ever released, `aicoded status` and `release_status` show E-MIG-005 on each, with the
  step, and the app's migrations become the steps that ran. Put a corrected version of the
  unreleased changes in `.aicoded/migration.sql`, leaving out the steps that ran, and publish
  again: the platform refuses a publish whose draft repeats one of them (E-MIG-001). `aicoded dev`
  then makes the dev database again from the app's migrations and the new draft.
- When the step could not finish, through a lock wait, a deadlock, a time limit or a broken
  connection, nothing is rejected: the publish shows E-MIG-005 as a note, and the next release
  tries the step again.
- When the step ran but was not recorded, since the connection broke right after it ran and its
  row could not be added to the journal even on a new connection, nothing is rejected either: the
  publish shows E-MIG-005 as a note, and the platform's operator records the step before the next
  release. Nothing in the app needs to change.
- A rollback to an older release, whose steps the database ran already, applies nothing.
- A release whose migrations do not start with what the database ran, such as one from another
  line of publishes, is refused (E-MIG-004), and so is a release of a publish whose schema changes
  were rejected.

## A published change never changes

Once a publish passed, its statements are part of the app's migrations as they are, and every
database of the app runs them in that order. A mistake in one is fixed by the next draft, such
as a new column in place of a wrong one, never by editing what was published. Editing
`.aicoded/migrations.json` changes nothing on the platform, and `aicoded dev` replaces it with
the platform's answer the next time it asks.

## See also

- [The SQL database](sql-database.md): queries, transactions and the database in `aicoded dev`.
- [Publishing](publishing.md): what a publish sends and how to read its outcome.
- [Rules for app code](rules.md): E-LINT-013 and the other rules.
