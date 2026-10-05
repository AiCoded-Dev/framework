# E-MAN-010: SQL database not declared

The app opened its SQL database, but its permission list (`aicoded.yaml`) does not declare it. The database is declared under `data` with the classes of data it holds, so security sees what the app stores.

**Fix:** add `- source: sqldb` with `classes: [...]` to `data` in `aicoded.yaml`.
