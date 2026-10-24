# E-REL-018: app's resources at another target

The platform makes each app's database, file buckets and mailbox at the target of the server its
organisation is placed on, and keeps using them at every later release. This app's resources were
made at another target than the one of the organisation's server now, and moving an app's data
between targets is not done yet. Rather than release the app without its data, the platform refuses
the release. Nothing was released.

**Fix:** ask the platform's operator to place the organisation on a server of the target the app's resources are at, then release again
