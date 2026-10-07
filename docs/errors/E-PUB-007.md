# E-PUB-007: the commit is already published

Every publish needs a new commit: the app has published this commit already, and that publish keeps its outcome and its change record. `aicoded publish` refuses before sending when the commit is the app's last publish or its base, and the platform refuses any other commit the app has published.

**Fix:** commit a change, then run `aicoded publish` again; to run the checks again on the same code, commit with `git commit --allow-empty`.
