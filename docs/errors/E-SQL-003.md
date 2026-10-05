# E-SQL-003: database client refused

The app's MySQL client asked for TLS, speaks a protocol older than MySQL 4.1, or sent a handshake the runner cannot read. The connection to the runner never leaves the machine and needs no TLS, and old clients are not supported.

**Fix:** open the database with `sqldb.Open`, which sets up the client correctly.
