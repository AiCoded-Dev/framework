// Package migrate checks a schema-change draft, .aicoded/migration.sql, and applies an app's
// migrations to its database as the database's owner.
//
// A draft holds at most 64 KiB of UTF-8 and 100 statements. Check splits it into statements and
// allows only those that add to the schema of the app's own database (Classify): CREATE TABLE,
// CREATE INDEX, and ALTER TABLE with only ADD clauses, where a new NOT NULL column has a
// DEFAULT. Destructive statements are refused with E-MIG-003 and everything else with
// E-MIG-002. Each publish adds its draft's statements to the app's list of steps, and Apply
// runs the steps that the database's journal, the table aicoded_migrations, lacks.
//
// The aicoded CLI and the platform import it. App code may not: E-LINT-001 refuses the import.
package migrate
