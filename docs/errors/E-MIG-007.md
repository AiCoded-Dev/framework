# E-MIG-007: a rollback across a destructive schema change

A release of an older publish, a rollback, applies no schema change: the database keeps the steps it ran, and the older version runs on them. That works while the steps after the older publish only added to the schema. Here they hold a destructive one (E-MIG-003), such as a dropped column or a renamed table, that ran in production. The older version still uses what that step removed or changed, and the data it removed cannot come back, so the release is refused and the version that runs keeps serving.

To undo what a newer version did, change the code and the draft and publish again: a new column or table in place of what was dropped, with its data filled from the app's code.

**Fix:** release a newer publish; a rollback cannot undo a destructive change
