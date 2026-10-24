# E-REL-011: building block not offered where the app runs

The platform makes each app's database, file buckets, mailbox and route where the organisation's
apps run: at the target of the server the platform's operator placed the organisation on. That
target does not offer one of the building blocks the app's permission list declares, or the
platform cannot reach the target at all, and then it offers none. Rather than release an app that
would miss a building block, the platform refuses the release. Nothing was released.

**Fix:** remove the building block from aicoded.yaml and the code that uses it, run aicoded check, commit and publish again; or ask the platform's operator to place the organisation on a server whose target offers it, or to make the target reachable, then release again
