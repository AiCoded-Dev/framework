# E-PUB-001: the app's folder is not the top level of a git repository with a commit

`aicoded publish` sends a commit of the app's own git repository, so the app's folder, the one that holds `aicoded.yaml`, must be the top level of a git repository whose `HEAD` is a commit with its whole history. Here it is not, and the message says why: the folder is in no git repository; it is a folder inside a bigger repository, whose whole history, with code that is not the app's, a publish would send; the repository has no commit yet; it is a shallow clone, which lacks the start of its history; or it names its commits with SHA-256, which the platform does not read yet. Nothing was sent.

**Fix:** make the app's folder a git repository of its own, with `git init`, `git add -A` and `git commit` in it; in a shallow clone, run `git fetch --unshallow`.
