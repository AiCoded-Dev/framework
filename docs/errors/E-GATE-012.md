# E-GATE-012: a step ran out of time, memory or disk

The delivery pipeline stopped a step because it ran longer than its time limit, used more memory than its limit, or left too little free disk, as the message says. Tests that never end, or that use much memory or write large files, cause this. When one of the security checks runs over its limits instead, its fix reads: the app is too large for the security checks on the platform: make it smaller, or tell your administrator.

**Fix:** make the tests end sooner and use less memory and disk, commit, and publish again; if small tests cause it, tell your administrator.
