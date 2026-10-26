# E-MIG-006: a draft without a database

The app has a draft, `.aicoded/migration.sql`, but its permission list, `aicoded.yaml`, declares no `sqldb`, so the app has no database to change.

**Fix:** declare sqldb in aicoded.yaml when the app keeps data in a database, or delete .aicoded/migration.sql
