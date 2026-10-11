# E-REL-009: publish still released

The platform's operator asked to purge the bundle of a publish that a release still runs: the
release is live, or pending on its server. The server needs the publish's built app whenever it
starts the app again, so the platform purges nothing of a publish that a live or pending release
names. The message names the release. Nothing was purged.

**Fix:** rotate the leaked secret, release a publish of the app without it so that it supersedes the release the message names, and purge again
