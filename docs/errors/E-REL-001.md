# E-REL-001: publish not ready for release

The platform releases a publish to production only when the delivery pipeline passed it: it built
the commit, every security check passed, and it kept the built app. This publish did not pass,
was refused, ended in an error, is still running, or has no built app, such as one made before
the pipeline kept them; or its permission list does not pass the platform's checks, as when it was
written for a newer framework than the platform's. Nothing was released, and the app keeps running
the release it had.

A rollback, which releases again the publish of an earlier release, is refused with this code too
when that release never went live, so there is nothing to restore, or when it is the app's live
release already. Roll back only to a release that went live and was replaced since.

**Fix:** run aicoded status for the publish, fix what the checks found, publish again with aicoded publish, and release the publish that passes
