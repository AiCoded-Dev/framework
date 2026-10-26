# E-MIG-004: migrations that differ from what the database ran

A database keeps the steps of migrations it ran in its journal, the table `aicoded_migrations`: their place, publish, place in the publish and hash. A release's list of steps must start with the journal's, or, for a rollback to an older release, be the start of it. Here the two differ: the release comes from another line of publishes than the one live, or someone changed the database or its journal by hand. A list whose steps are not numbered from 1 or whose hash does not match its statement is refused the same way. Nothing was applied. In `aicoded dev` the dev database is made again instead, since its data is test data.

**Fix:** release a publish made on top of the one live, so that its migrations start with the ones the database ran; a database changed by hand needs the platform's operator
