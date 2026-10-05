# E-RPC-010: call with no viewer to a function that needs one

The call was made from a context with no viewer, such as the one `OnStart` receives, so it went as the calling app itself. A function accepts such calls only when its `//ssr:access` line has `apps=true`. The runner and the called app both refuse it otherwise.

**Fix:** make the call with the context of the viewer's request, or, if the app itself may call the function, add `apps=true` to its `//ssr:access` line in the called app and run `aicoded generate` there.
