# E-SQL-001: database command not allowed

The app's database connection runs queries and prepared statements only. Changing the user, changing connection options, replication commands and other administrative commands are refused, and the connection ends. Packets sent out of order are refused the same way.

**Fix:** use the `*sql.DB` from `sqldb.Open` for queries and statements; the database user and options are set by the platform.
