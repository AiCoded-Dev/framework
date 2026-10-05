// Package web serves an app's pages. Pages are templates under pages/, and aicoded generate
// turns them into code that uses this package for routing, access rules, forms, live values and
// page calls. App code uses the rest: the [Request] and [ResponseWriter] its hooks get, the
// errors that end a request, and [SafeHTML].
//
// Each page has a data provider, DP in its dataprovider.go. For a request, the hooks of the
// pages on the path run root first, in this order: the access rules of <ssr:access>, then every
// Guard, then, on a post, the check of the form's token, and then Init<Form>, Process<Form>
// (only for the posted form, and only when all its fields are valid) and Data of the layouts on
// the path and of the page itself. A live connection and a page call pass the same access rules
// and Guards first.
//
// A hook ends a request with [NotFound], [Forbidden], [Error] or [Redirect]. Only the message of
// an [Error] reaches the viewer. Any other error answers 500 "Something went wrong." and is
// logged, with its text under aicoded dev in environment `dev` and only its kinds and codes
// anywhere else. [Redirect] goes only to a path on this site (E-WEB-002).
//
// {{ }} in a template escapes a value for where it lands: text, an attribute or a URL. {{$ }}
// writes markup as it is and takes only a [SafeHTML], made with [HTMLConst], [EscapeHTML] or
// [JoinHTML]. Every response carries a strict Content-Security-Policy and the other security
// headers, pages are never cached, and a form posted from another site or without its token is
// refused with 403.
//
// Read more in the guides docs/guides/web-api.md, docs/guides/request-pipeline.md,
// docs/guides/routing.md and docs/guides/access.md, which aicoded explain and the MCP tool howto
// print as guides/web-api, guides/request-pipeline, guides/routing and guides/access.
package web
