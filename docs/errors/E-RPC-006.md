# E-RPC-006: the client package of a called app cannot take its name

An app calls another app through a generated package in `services/`, named after the called
app without its dashes: the client of `billing-eu` is the package `billingeu` in
`services/billingeu/`. `aicoded generate` refuses a called app whose package name Go reserves,
a keyword such as `go` or `type`, a predeclared name such as `string` or `len`, `main` or
`init`, and two called apps whose package names are the same, such as `a-b` and `ab`.
`aicoded rpc add` refuses such an app, and a name that is not an app name, before it writes
anything.

**Fix:** rename the called app in its aicoded.yaml so that its name without dashes is neither reserved by Go nor the package name of another app this app calls, then, in the calling app, remove its old `.aicoded/services/<app>.json` if there is one and run `aicoded rpc add <new name>`.
