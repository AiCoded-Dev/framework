# E-RPC-007: aicoded rpc add found no app to call

`aicoded rpc add <app>` runs in the folder of the calling app. It looks for `<app>` among the
apps in the parent folder of that folder and below, by the `app:` line of each `aicoded.yaml`,
generates it, and copies its snapshot into the calling app. It refuses a name that no app there
has, a name that two apps there have, the calling app itself, and an app that serves no
function: one without an `rpc/` package, or whose `rpc/` package has no exported function.

**Fix:** keep the called app's folder next to the calling app's folder, name another app as the `app:` line of its aicoded.yaml does, and give it an rpc/ package with the functions to call.
