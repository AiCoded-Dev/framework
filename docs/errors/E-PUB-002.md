# E-PUB-002: changes that are not committed

A publish sends a commit, not the files as they are on disk, so nothing may be left uncommitted: `git status --porcelain` must print nothing, untracked files included. Here git lists the files the message names, up to five, each after the letters git gives its state, such as `M` for a changed file and `??` for an untracked one. Files that `.gitignore` names do not count, and are never sent. Nothing was sent.

**Fix:** commit the files, or remove them, then run `aicoded publish` again.
