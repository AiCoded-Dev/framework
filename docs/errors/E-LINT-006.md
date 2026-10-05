# E-LINT-006: database opened directly

An app reaches its SQL database only through the runner, which logs in to it as the app's own user, so the app never holds a password or the address of a database server. App code may not open a database with `sql.Open` or `sql.OpenDB`, register a driver with `sql.Register`, or reach the driver of a database with the `Driver` method of `*sql.DB`, whose driver can open connections of its own; a method of an interface with the name and signature of `Driver`, where a type parameter in its signature stands for any type, counts as `Driver`. `sqldb.Open(ctx)` returns the app's database as a `*sql.DB`, and its other methods and the types of `database/sql` stay allowed. Generated files are checked too, since they hold the code of the templates' expressions, and a problem there points to the template line that the code was written from.

**Fix:** get the database with sqldb.Open(ctx) in OnStart and pass the *sql.DB it returns.
