// Package app runs an app under its runner. An app's main calls [Main] with its [Options]:
//
//	func main() {
//		d := &deps.Deps{}
//		app.Main(app.Options{Handler: pages.NewHandler(d), OnStart: d.Start})
//	}
//
// Handler serves the app's pages: pass the pages.NewHandler that aicoded generate writes. RPC
// serves the functions of the app's rpc/ package to other apps: pass the rpc.Server() that
// aicoded generate writes there. An app needs one or both. OnStart runs once, before the app
// serves: open the database and file stores there, and keep them in the app's deps package for
// every page. aicoded check holds main to this form: the options are an [Options] literal
// written in the call of [Main] or [Run], with Handler and RPC set to those calls (E-LINT-004).
//
// The runner starts the app with one environment variable, AICODED_RUNNER_DIR, and talks to it
// only through sockets in that folder. [Run] greets the runner, which refuses an app built for
// another protocol version (E-RUN-002), and then serves only requests that carry a viewer the
// company login verified, so an app has no login code. Every request gets a trace span, a panic
// answers 500 and is logged while the app keeps serving, and the default slog logger writes JSON
// lines that the runner collects. [Main] stops on SIGINT or SIGTERM and lets the requests in
// flight finish.
//
// [Name], [Env] and [Version] tell the app its name, the environment it runs in and its release.
//
// Read more in the guides docs/guides/overview.md and docs/guides/project-structure.md, which
// aicoded explain and the MCP tool howto print as guides/overview and guides/project-structure.
package app
