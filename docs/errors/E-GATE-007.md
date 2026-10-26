# E-GATE-007: a framework version the platform does not build

The delivery pipeline builds apps with the framework versions it knows, which the message lists. `go.mod` requires another version of `aicoded.dev/framework`, or does not require the framework at all.

The platform builds apps with v0.6.0 and later only. From v0.6.0 on, an app's schema changes travel with its publishes in the draft `.aicoded/migration.sql`, and the app's own database user reads and writes rows only, so an older app that creates its tables in its code would fail on the platform. For an app older than v0.6.0 the fix says so: require v0.6.0 or a later listed version, and move every `CREATE TABLE` and other schema change from the app's code to `.aicoded/migration.sql`. See [schema changes](../guides/schema-changes.md).

**Fix:** require one of the listed versions with `go get aicoded.dev/framework@<version>`, run `go mod tidy`, and commit; for an app older than v0.6.0, require aicoded.dev/framework v0.6.0 or a later listed version with `go get aicoded.dev/framework@<version>`, run `go mod tidy`, move DDL from the app's code to `.aicoded/migration.sql`, and commit.
