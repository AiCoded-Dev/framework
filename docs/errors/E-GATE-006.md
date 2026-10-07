# E-GATE-006: a module that the app may not use

An app may use the framework, the third-party modules its permission list declares in `modules:`, and the modules that those depend on, and no other. `go.mod` requires a module that is none of these, at the line of the problem.

**Fix:** declare the module in `modules:` of `aicoded.yaml`, or remove it from the code and run `go mod tidy`.
