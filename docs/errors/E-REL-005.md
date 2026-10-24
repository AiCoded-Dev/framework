# E-REL-005: settings or secrets not kept where the app runs

The platform keeps an app's secrets only where the organisation's apps run, at the target of the
server the platform's operator placed the organisation on, and only when that target keeps them. It
keeps no app's settings yet. Rather than run the app without them, it refuses to release an app
whose permission list declares `settings`, or declares `secrets` where the target does not keep
them. Nothing was released.

**Fix:** remove `settings` from aicoded.yaml and the code that reads them, and `secrets` too unless the platform's operator places the organisation on a server whose target keeps them; run aicoded check, commit, and publish again
