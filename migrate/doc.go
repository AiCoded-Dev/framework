// Package migrate checks a schema-change draft, .aicoded/migration.sql, and applies an app's
// migrations to its database as the database's owner.
//
// A draft holds at most 64 KiB of UTF-8 and 100 statements. Check splits it into statements and
// allows only those that change the schema of the app's own tables (Classify): Expand ones, which
// add to it, CREATE TABLE, CREATE INDEX, and ALTER TABLE with only ADD clauses, where a new NOT
// NULL column has a DEFAULT; and Destructive ones, in a fixed set of DROP, RENAME, TRUNCATE and
// ALTER TABLE forms, which Notes names with E-MIG-003. Everything else is refused with E-MIG-002.
// Each publish adds its draft's statements to the app's list of steps, and Apply runs the steps
// that the database's journal, the table aicoded_migrations, lacks, a destructive one only when
// its caller allows it.
//
// The aicoded CLI and the platform import it. App code may not: E-LINT-001 refuses the import.
package migrate
