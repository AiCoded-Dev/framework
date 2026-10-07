# E-PUB-014: the platform could not read the commit

The publish ended in `refused`: the delivery pipeline verifies the git bundle of each publish and takes the commit out of it, and it could not. The bundle was not a valid git bundle, lacked objects that its commit needs, or did not hold the commit it named. The message quotes the reason, and the publish's change record says so too. A repository that git itself finds damaged gives this.

**Fix:** make sure `git fsck` finds nothing wrong in the app's repository, commit again, then run `aicoded publish`.
