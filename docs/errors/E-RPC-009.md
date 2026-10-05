# E-RPC-009: function not served to the calling app

The called app does not list the calling app among the callers of this function. Each exported function in an app's `rpc/` package names the apps that may call it with `caller=` on its `//ssr:access` line, and `aicoded generate` copies them into `services.serves` of the permission list. The runner and the called app both refuse a call from any other app.

**Fix:** add the calling app to `caller=` on the function's `//ssr:access` line in the called app, then run `aicoded generate` there.
