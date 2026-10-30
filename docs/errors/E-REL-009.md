# E-REL-009: publish still released

The platform's operator asked to purge the bundle of a publish that a release or the app's preview
still runs: the release is live, or pending on its server, or the preview is running. The server
needs the publish's built app whenever it starts the app again, so the platform purges nothing of a
publish that a live or pending release, or a running preview, names. The message names the release
or the preview. Nothing was purged.

**Fix:** rotate the leaked secret, release a publish of the app without it so that it supersedes the release the message names, or have the app's owner end the preview or preview a publish without it, and purge again
