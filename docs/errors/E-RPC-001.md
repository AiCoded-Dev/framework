# E-RPC-001: a function in rpc/ has no access line

Other apps can call every exported function of an app's `rpc/` package, so each one must say
who may call it. `aicoded generate` refuses an exported function without an `//ssr:access`
line in its doc comment, and writes nothing until every such function has one. The line names
the calling apps and whether a call may come on behalf of a viewer, with the roles the viewer
needs, or from the calling app itself with no viewer (E-RPC-002). Unexported functions and
methods cannot be called from other apps: they need no line, and a line on them is refused
(E-RPC-002).

**Fix:** add `//ssr:access caller=<app> role=<role>` on the line right above the function, with `apps=true` in place of or next to `role=`, or unexport the function.
