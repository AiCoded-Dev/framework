// Package auth tells who is viewing: the person the company login verified, whom the runner
// forwards with every request. An app has no login code and never sees a password.
//
// [Viewer] returns the viewer in every hook of a page, and in a function of the app's rpc/
// package, where it is the viewer on whose behalf the other app calls. It returns the zero
// [Identity], which has no roles, outside a request, such as in OnStart, and in a call another
// app makes as itself.
//
// <ssr:access role="..."/> in the templates sets which pages a viewer may open, and the
// //ssr:access lines in rpc/ which functions they may call. Which records they may see is up to
// the app:
//
//   - Store the viewer's Subject, a stable id, as a record's owner. Name is for showing only.
//   - Check ownership in the page's Guard, declared with guard="true": return web.Forbidden()
//     for a record the viewer may not see, or web.NotFound() only when even its existence must
//     stay hidden.
//   - A function of rpc/ that returns records checks the same rule again.
//
// Read more in the guide docs/guides/access.md and the task docs/tasks/ownership-check.md, which
// aicoded explain and the MCP tool howto print as guides/access and tasks/ownership-check.
package auth
