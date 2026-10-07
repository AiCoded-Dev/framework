# E-PUB-009: the publish is too large

A publish sends at most 32 MiB: a git bundle of the commit with its history since the app's base, or with all of it on the first publish or after the history was rewritten. This one is larger. A file stays in the history once committed, even after it is deleted, so a large file committed once keeps every bundle of all the history large. `aicoded publish` refuses before it sends anything, and the platform refuses a larger upload too.

**Fix:** remove large files from the repository and its history, or publish from a new repository that holds only the app.
