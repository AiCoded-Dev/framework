# E-RUN-016: the release is not signed by the platform

The server's runner refused to start a release or a preview of the app: it carries no signature,
a signature by no key the runner trusts, or a signature that does not match the release, such as
one for another built app, another app or organisation, the other environment, another permission
list, resource limit or schema change. Only what the platform signed runs on a server, so nothing
of this release was started. A runner that trusts no key at all refuses to start, with this code.

**Fix:** Ask the platform operator; the control plane must be upgraded before the runner, and the runner's package must carry the control plane's current public key.
