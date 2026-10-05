# E-SQL-002: runner could not open the app's database

The app reached the runner, but the runner could not log in to the database as the app's user: the database server is down, busy, or the app's database has not been prepared. The runner's log says why.

**Fix:** in `aicoded dev`, check that the MySQL server in `dev.yaml` runs and read the runner's log line about the database; on the platform, retry and report it if it stays.
