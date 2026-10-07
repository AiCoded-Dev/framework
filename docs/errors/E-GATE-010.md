# E-GATE-010: the commit is too large

The files of the commit, once taken out of the bundle, are more than the delivery pipeline takes: at most 20,000 files and 256 MiB in all.

**Fix:** remove the files the app does not need, such as data, build output or copies of modules, and commit.
