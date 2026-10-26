// Package sqldb opens the app's SQL database, a MySQL database of its own. The app never holds a
// credential: [Open] connects to the runner's socket, and the runner logs in to the database as
// the app's own user, whose grants reach only the app's database. There is no DSN and no
// password anywhere in the app.
//
// The permission list declares the database as a data source:
//
//	data:
//	  - source: sqldb
//	    classes: [internal]
//
// The app's tables come from its schema changes, never from its code. Write the next change in
// the draft .aicoded/migration.sql, which stays out of git and goes to the platform with
// aicoded publish; aicoded dev, aicoded check and the platform apply it as the database's owner.
// The app's own user reads and writes rows only, and aicoded check refuses CREATE, ALTER, DROP,
// RENAME and TRUNCATE in app code (E-LINT-013).
//
// Rules:
//
//   - Open the database once, in OnStart, and share the *sql.DB with the pages through the app's
//     deps package.
//   - Pass every value with a ? placeholder; never join values into SQL.
//   - Run statements that belong together in a transaction, and send mail about them only after
//     it commits.
//   - One statement runs per call, and times are read and written in UTC.
//
// Every query and transaction gets a trace span that names only the statement's first keyword,
// never its text or its values.
//
// A test opens the test database that aicoded check makes, with the app's tables, through
// package aicoded.dev/framework/sqldb/sqltest.
//
// Read more in the guides docs/guides/sql-database.md and docs/guides/schema-changes.md and the
// task docs/tasks/list-from-database.md, which aicoded explain and the MCP tool howto print as
// guides/sql-database, guides/schema-changes and tasks/list-from-database.
package sqldb
