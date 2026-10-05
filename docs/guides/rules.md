# Rules for app code

The rules that `aicoded check` holds app code to: what an app must not do, what it does
instead, and the error code of each rule. Lint runs once an app builds and passes `go vet`, and
nothing in the code turns a finding off.

## How lint reads an app

- It reads every Go file of the app except the tests, `_test.go` files, which never reach the
  binary.
- A file counts as generated only when `aicoded generate` wrote it for the app. Generated files
  may also import and call what only generated code uses, such as `web/render` and `web.New`;
  every other file is hand-written, whatever its name or its first line.
- Templates are app code too. Their expressions become Go code in the generated files, which
  E-LINT-006, E-LINT-007, E-LINT-008 and E-LINT-009 check as they check hand-written files, and a
  problem there points to the template's line, such as `pages/index.html:12`.
- No comment turns a finding off: `//nolint`, `#nosec` and the like change nothing.
- It also reads the code of the third-party modules that `modules:` in the permission list
  declares, as far as the app's build uses it (E-LINT-003).
- Each finding has its code, `file:line` and a one-line fix. `aicoded explain <code>` prints the
  rule's page, with the full list where the rule has one: the page of E-LINT-001 lists every
  standard package an app may import, and the page of E-LINT-009 every name of `net/http` and
  `database/sql` it may use.

## The rules

<!-- rules -->
| Code | Rule | Don't | Do |
|---|---|---|---|
| [E-LINT-001](../errors/E-LINT-001.md) | import not allowed | Import os, net, reflect, unsafe, runnerproto or any other package that is not on the list of allowed imports. | Reach the database, files, mail, settings and secrets through the building blocks, and import only your own packages, the building blocks, the allowed standard packages and the packages of the modules that modules: declares. |
| [E-LINT-002](../errors/E-LINT-002.md) | module not declared | Import a package of a third-party module that modules: in aicoded.yaml does not list, directly or through a module it lists. | List every third-party module your code, or a module you list, imports in modules: in aicoded.yaml. |
| [E-LINT-003](../errors/E-LINT-003.md) | declared module uses a banned capability | Declare a module that runs programs, opens network connections, makes system calls, uses unsafe, //go:linkname or assembly, reaches a method or the database driver through reflect, imports the framework, opens a database, runs SQL, replaces the default logger or changes a variable of the standard library or the framework. | Declare only modules of plain Go that compute, and reach everything else through the building blocks. |
| [E-LINT-004](../errors/E-LINT-004.md) | handler not generated | Pass app.Main or app.Run options other than an app.Options literal written in the call, use either as a value, pass app.Options a raw http.Handler, a wrapped handler or an rpc.Server you build yourself, write a composite literal that builds a value of a type parameter, or set a Handler or RPC field whose type has a type parameter. | Call app.Main with an app.Options literal written in the call that passes Handler: pages.NewHandler(...) and RPC: rpc.Server(), the functions aicoded generate writes. |
| [E-LINT-005](../errors/E-LINT-005.md) | for generated code only | Call rpc.Call, web.New, a form field's Process or another function meant for generated code, or use an unexported name of a generated file. | Call another app through its generated client in services/, and use only the exported names of generated files. |
| [E-LINT-006](../errors/E-LINT-006.md) | database opened directly | Open a database with sql.Open or sql.OpenDB, register a driver with sql.Register, or reach the driver with DB.Driver. | Get the app's database with sqldb.Open(ctx), which reaches it through the runner. |
| [E-LINT-007](../errors/E-LINT-007.md) | SQL is not a constant | Build SQL by joining strings, with fmt.Sprintf or from variables, pass a method of database/sql that takes SQL around as a value, put more than one statement in one call, or read or write a file of the database server. | Write every statement as a constant query or a simple table change, pass every value with a ? placeholder, and pass a list for IN as one JSON value that JSON_TABLE reads. |
| [E-LINT-008](../errors/E-LINT-008.md) | personal data or a secret in a log or span | Put a form value, a URL parameter, the request, the viewer's identity, a value from the database, a mail, a stored file, a secret, another app's answer or the input of a page call, live value or RPC function into a log line, a print or a span, as it is or in a string, error or struct made from it. | Log and span only ids, counts, codes and constant text, and return errors instead of logging them: the framework records them without their text. |
| [E-LINT-009](../errors/E-LINT-009.md) | name not allowed | Use the parts of net/http and database/sql that serve, call out, read files or reach the driver, use reflect through a value, replace the default logger, change a variable of the standard library or the framework, or a value of their types or of a type parameter through a pointer, or convert to or from a type parameter that could be a pointer. | Use the types and constants of net/http and database/sql around the building blocks, log with log/slog as the framework sets it up, and keep your own variables. |
| [E-LINT-010](../errors/E-LINT-010.md) | file the checks cannot read | Add assembly, C or object files, cgo, Go files that a //go:build line or a name such as x_windows.go leaves out of the build, or Go files in a folder that go.mod ignores. | Write the app in plain Go files that are built on every machine. |
| [E-LINT-011](../errors/E-LINT-011.md) | replace, go.work or vendor not allowed | Replace a module in go.mod with a folder or another module, build the app with a go.work file or a vendor folder, edit a module in the module cache, or set any GOFLAGS entry outside the short list lint allows, such as -overlay, -modfile, -mod=vendor or -toolexec. | Require released versions of the framework and of the modules that modules: declares; only an aicoded installed from a framework checkout accepts the replace line its aicoded init writes. |
<!-- end rules -->
