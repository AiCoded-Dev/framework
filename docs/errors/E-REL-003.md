# E-REL-003: framework too old

The runner on the platform releases apps built with `aicoded.dev/framework` v0.6.0 and later: from v0.6.0 on, an app's schema changes travel with its publishes, in the draft `.aicoded/migration.sql`, and the platform applies them as the database's owner while the app gets a user that reads and writes rows only, so an older app that creates its tables in its code would fail there. The publish was built with an older version of the framework, or with a version the platform cannot read, as the `go.mod` of its commit requires it. Nothing was released.

**Fix:** require `aicoded.dev/framework` v0.6.0 or later in go.mod, such as with `go get aicoded.dev/framework@latest`, and move DDL to .aicoded/migration.sql, run aicoded generate and aicoded check, commit, and publish again
