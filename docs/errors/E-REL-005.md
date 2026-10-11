# E-REL-005: settings and secrets not kept yet

The platform does not keep the values of an app's settings and secrets yet. Rather than run the
app without them, it refuses to release an app whose permission list declares `settings` or
`secrets`. Nothing was released.

**Fix:** remove `settings` and `secrets` from aicoded.yaml and the code that reads them, run aicoded check, commit, and publish again
