# E-PUB-007: the commit is already published

Every publish needs a new commit: the app has published this commit already, and that publish keeps its outcome and its change record. `aicoded publish` refuses before sending when the commit is the app's last publish or its base, and the platform refuses any other commit the app has published.

When that publish passed and is held for its owner's preview, it needs no new commit, and the message says `this commit is already published to the app as publish <id>, which is held for its owner's preview`, with the fix `preview it with aicoded preview <id>, and release it only when the owner asks: aicoded release <id>`.

**Fix:** commit a change, then run `aicoded publish` again; to run the checks again on the same code, commit with `git commit --allow-empty`.
