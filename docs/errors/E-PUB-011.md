# E-PUB-011: the delivery pipeline could not finish

The publish ended in `error`: the delivery pipeline could not finish it, through a fault of its own, such as a worker that stopped or a step whose output it could not read. The message quotes the platform's reason, and the publish's change record says so too. The commit is not at fault, but a commit the app has published cannot be published again (E-PUB-007).

**Fix:** commit again, even with `git commit --allow-empty`, then run `aicoded publish`; if it fails again, give your administrator the publish's id.
