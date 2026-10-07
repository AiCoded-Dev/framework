# E-GATE-001: replace line in go.mod

`go.mod` has a `replace` line, which builds a module from somewhere other than its released version, such as a folder on your computer or a changed copy, so the checks would not read the code that runs. The delivery pipeline refuses every `replace`, also the one of the framework that `aicoded init` writes from a framework checkout. The problem is at the line.

**Fix:** remove the `replace` line, and require a released version of the module.
