# E-DEV-006: no MySQL server for an app that declares sqldb

The app declares its SQL database (`- source: sqldb` in `aicoded.yaml`), but `dev.yaml` names no MySQL server for `aicoded dev` to create it on. `aicoded dev` uses a MySQL 8 server on your machine and creates one database and user per app on it.

**Fix:** add `mysql: user:password@unix(/var/run/mysqld/mysqld.sock)/` (the admin DSN of a local MySQL 8) under `workspaces.<this directory>` in `dev.yaml`.
