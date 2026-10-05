# E-DEV-008: app database could not be prepared

`aicoded dev` could not create or update the app's database and user (both named `ac-dev-<app>`) with the admin user from `dev.yaml`. The message gives the MySQL error number, the network error, or that the server did not answer within 30 seconds.

**Fix:** check that MySQL runs, and that the admin user has `CREATE USER` and `ALL PRIVILEGES ... WITH GRANT OPTION` on the apps' databases, in practice on `*.*`.
