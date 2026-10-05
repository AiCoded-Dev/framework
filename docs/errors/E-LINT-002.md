# E-LINT-002: module not declared

An app may use a third-party module only when its permission list declares it, so that what the app is built from is visible and its code is checked (E-LINT-003). The app's own packages, or the packages of a module the app declares, import a package of a module that `modules:` in `aicoded.yaml` does not list. The problem stands at the module's `require` line in `go.mod`. The framework needs no declaration, and neither do the modules that only the framework imports, such as the protobuf runtime, nor a module that only test files import. A module the framework requires still needs declaring when the app's code or a declared module imports it. A module that a `go.work` file brings in is never a declared module, whatever `modules:` lists, and E-LINT-011 refuses the `go.work` file itself.

**Fix:** add the module to modules: in aicoded.yaml, or stop importing its packages.
