# E-RUN-009: access of an unknown kind

The platform makes each app's database, file buckets and mailbox at the target of its server, and
hands the server's runner one grant for each: the access that resource gives. The runner was handed,
for this release, a grant of a kind it does not know, or two grants of one kind, as when the
platform is newer than the server's runner. Rather than guess which resource the grant is for, the
runner refuses the release before it starts anything. Nothing was started, and the release that
was live before keeps serving.

**Fix:** ask the platform's operator to update the runner of the organisation's server, then release the app again
