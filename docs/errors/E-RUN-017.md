# E-RUN-017: the release is older than one the server has served

The server's runner refused to start a release whose signature is older than the newest signature
of a release this server has served for the app in that environment, production or preview. A
release the platform signed earlier cannot be sent again to take the place of a newer one. A
rollback is a new release with a signature of its own, so it is not refused. Nothing was started.

**Fix:** Release again; an old release cannot be replayed.
