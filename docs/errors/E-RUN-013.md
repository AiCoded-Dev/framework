# E-RUN-013: the app is stopped after a schema change

The app's new version ran a schema change that removed or changed something, such as a dropped
column, and then did not start. The earlier version still uses what that change removed, so the
runner does not start it on the changed schema: it would fail, or write data in the wrong place.
The runner tried to start the new version 3 times, 5 seconds apart, and reported the release
failed. Until a version that runs on the changed schema goes live, the app's address answers this
page with status 503, and the app serves no one. See [schema changes](../guides/schema-changes.md).

**Fix:** publish a fixed version; the app stays stopped until it goes live
