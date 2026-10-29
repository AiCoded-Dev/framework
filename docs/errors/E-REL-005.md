# E-REL-005: secrets not kept where the app runs

The platform keeps an app's secrets only where the organisation's apps run, at the target of the
server the platform's operator placed the organisation on, and only when that target keeps them.
Rather than run the app without them, it refuses to release an app whose permission list declares
`secrets` where the target does not keep them. Nothing was released. Settings are kept on every
target; a declared setting or secret with no value is E-REL-022.

**Fix:** remove `secrets` from aicoded.yaml and the code that reads them, unless the platform's operator places the organisation on a server whose target keeps them; run aicoded check, commit, and publish again
