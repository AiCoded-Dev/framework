# E-GATE-012: a step ran out of time, memory or disk

The delivery pipeline stopped a step because it ran longer than its time limit, used more memory than its limit, or left too little free disk, as the message says. In the tests step, tests that never end, or that use much memory or write large files, cause this. In the steps of the security checks and in the build, the app is too large for the limits the platform gives them; in the simulated attacks, the running app itself used more memory than its limit. In the modules step, the modules the app uses fill the disk.

**Fix:** the one for the step that ran over:

- the modules step: use fewer or smaller modules, commit, and publish again; if the app's modules are few and small, tell your administrator;
- a security check or the build: the app is too large for the security checks on the platform: make it smaller, or tell your administrator;
- the tests: make the tests end sooner and use less memory and disk, commit, and publish again; if small tests cause it, tell your administrator.
