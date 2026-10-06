package lint

import "slices"

// Rule is one rule of lint, as the docs show it.
type Rule struct {
	Code  string // "E-LINT-001"
	Title string // the title of the rule's page, docs/errors/<Code>.md
	Dont  string // what app code must not do, in one sentence
	Do    string // what it does instead, in one sentence
	Fix   string // the one-line fix of every finding of the rule
}

var rules = []Rule{
	{
		Code:  "E-LINT-001",
		Title: "import not allowed",
		Dont:  "Import os, net, reflect, unsafe, runnerproto or any other package that is not on the list of allowed imports.",
		Do:    "Reach the database, files, mail, settings and secrets through the building blocks, and import only your own packages, the building blocks, the allowed standard packages and the packages of the modules that modules: declares.",
		Fix:   "remove the import and use a building block or an allowed standard package instead, which aicoded explain E-LINT-001 lists, or move a package of your own to a folder the checks read",
	},
	{
		Code:  "E-LINT-002",
		Title: "module not declared",
		Dont:  "Import a package of a third-party module that modules: in aicoded.yaml does not list, directly or through a module it lists.",
		Do:    "List every third-party module your code, or a module you list, imports in modules: in aicoded.yaml.",
		Fix:   "add the module to modules: in aicoded.yaml, or stop importing its packages",
	},
	{
		Code:  "E-LINT-003",
		Title: "declared module uses a banned capability",
		Dont:  "Declare a module that runs programs, opens network connections, makes system calls, uses unsafe, //go:linkname or assembly, reaches a method or the database driver through reflect, imports the framework, opens a database, runs SQL, replaces the default logger or changes a variable of the standard library or the framework.",
		Do:    "Declare only modules of plain Go that compute, and reach everything else through the building blocks.",
		Fix:   "remove the module from modules: and go.mod, and use a building block or a module that needs none of these",
	},
	{
		Code:  "E-LINT-004",
		Title: "handler not generated",
		Dont:  "Pass app.Main or app.Run options other than an app.Options literal written in the call, use either as a value, pass app.Options a raw http.Handler, a wrapped handler or an rpc.Server you build yourself, write a composite literal that builds a value of a type parameter, or set a Handler or RPC field whose type has a type parameter.",
		Do:    "Call app.Main with an app.Options literal written in the call that passes Handler: pages.NewHandler(...) and RPC: rpc.Server(), the functions aicoded generate writes.",
		Fix:   "call app.Main with an app.Options literal written in the call, with Handler: pages.NewHandler(...) and RPC: rpc.Server() directly in it, and serve every page from pages/",
	},
	{
		Code:  "E-LINT-005",
		Title: "for generated code only",
		Dont:  "Call rpc.Call, web.New, a form field's Process or another function meant for generated code, or use an unexported name of a generated file.",
		Do:    "Call another app through its generated client in services/, and use only the exported names of generated files.",
		Fix:   "remove the call or the name: the code aicoded generate writes makes that call, and app code uses the exported names of that code",
	},
	{
		Code:  "E-LINT-006",
		Title: "database opened directly",
		Dont:  "Open a database with sql.Open or sql.OpenDB, register a driver with sql.Register, or reach the driver with DB.Driver.",
		Do:    "Get the app's database with sqldb.Open(ctx), which reaches it through the runner.",
		Fix:   "get the database with sqldb.Open(ctx) in OnStart and pass the *sql.DB it returns",
	},
	{
		Code:  "E-LINT-007",
		Title: "SQL is not a constant",
		Dont:  "Build SQL by joining strings, with fmt.Sprintf or from variables, pass a method of database/sql that takes SQL around as a value, put more than one statement in one call, or read or write a file of the database server.",
		Do:    "Write every statement as a constant query or a simple table change, pass every value with a ? placeholder, and pass a list for IN as one JSON value that JSON_TABLE reads.",
		Fix:   "write the SQL as a constant and pass every value with a ? placeholder; when the viewer's role decides which rows to read, write one constant statement for each case and choose one in Go, as tasks/list-by-role shows; for a list in IN, use JSON_TABLE(?, ...) as guides/sql-database shows",
	},
	{
		Code:  "E-LINT-008",
		Title: "personal data or a secret in a log or span",
		Dont:  "Put a form value, a URL parameter, the request, the viewer's identity, a value from the database, a mail, a stored file, a secret, another app's answer or the input of a page call, live value or RPC function into a log line, a print or a span, as it is or in a string, error or struct made from it.",
		Do:    "Log and span only ids, counts, codes and constant text, and return errors instead of logging them: the framework records them without their text.",
		Fix:   "remove the value from the log line, print or span, or log an id, a count or a code instead",
	},
	{
		Code:  "E-LINT-009",
		Title: "name not allowed",
		Dont:  "Use the parts of net/http and database/sql that serve, call out, read files or reach the driver, use reflect through a value, replace the default logger, change a variable of the standard library or the framework, or a value of their types or of a type parameter through a pointer, or convert to or from a type parameter that could be a pointer.",
		Do:    "Use the types and constants of net/http and database/sql around the building blocks, log with log/slog as the framework sets it up, and keep your own variables.",
		Fix:   "use only the names that aicoded explain E-LINT-009 lists; return a value instead of writing it through a pointer to a standard-library or framework type, and keep values in variables of your own",
	},
	{
		Code:  "E-LINT-010",
		Title: "file the checks cannot read",
		Dont:  "Add assembly, C or object files, cgo, Go files that a //go:build line or a name such as x_windows.go leaves out of the build, or Go files in a folder that go.mod ignores.",
		Do:    "Write the app in plain Go files that are built on every machine.",
		Fix:   "delete the file, or make it a plain Go file with no `//go:build` line or `_<os>` or `_<arch>` name suffix that leaves it out of this build and no `import \"C\"`, in a folder that `go.mod` does not ignore",
	},
	{
		Code:  "E-LINT-011",
		Title: "replace, go.work or vendor not allowed",
		Dont:  "Replace a module in go.mod with a folder or another module, build the app with a go.work file or a vendor folder, edit a module in the module cache, or set any GOFLAGS entry outside the short list lint allows, such as -overlay, -modfile, -mod=vendor or -toolexec.",
		Do:    "Require released versions of the framework and of the modules that modules: declares; only an aicoded installed from a framework checkout accepts the replace line its aicoded init writes.",
		Fix:   "remove the replace line, the go.work file, the vendor folder or the GOFLAGS entries outside the list; run go clean -modcache when go mod verify fails; and require a released version of the module; when the go command itself cannot run, fix what it reports, such as the go line of go.mod or GOTOOLCHAIN",
	},
}

// Rules returns every rule, sorted by code.
func Rules() []Rule {
	return slices.Clone(rules)
}

// fix returns the fix of the rule code.
func fix(code string) string {
	i := slices.IndexFunc(rules, func(r Rule) bool { return r.Code == code })
	if i < 0 {
		panic("lint: no rule " + code)
	}
	return rules[i].Fix
}
