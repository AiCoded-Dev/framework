# E-CHK-002: Go does not compile

`aicoded check` compiles every package of the app with `go build`, and `aicoded dev` compiles the app before it starts it. The Go code at the position shown does not compile, or the go command could not load a package, for example because an import names a package that does not exist. When the go command names no place in a file, the message carries what it printed.

When an import names a package under `aicoded.dev/framework` that the framework does not have, the message also names the framework's packages for app code with the same last path element, and the fix imports them instead: for `aicoded.dev/framework/form`, it names `aicoded.dev/framework/web/form`. These are the framework's packages that `aicoded explain E-LINT-001` lists. The go command's own advice to run `go get` does not help there: the app already requires the framework, which has no such package.

**Fix:** fix the Go code at this line. When the problem has no line, fix what the go command reports in the message. When the message names a package of the framework, import that package instead.
