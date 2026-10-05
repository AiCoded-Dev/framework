// Package telemetry records trace spans. The runner exports them to the company's tools; in
// aicoded dev they show on the dev UI's Traces page.
//
// The app already gets a span for every request it serves and every call it makes to another
// app, and for every query, file and mail operation of the building blocks. Add a span with
// [Start] around a step of the app's own that takes time:
//
//	ctx, span := telemetry.Start(ctx, "import rows")
//	defer span.End()
//
// [Span.SetAttr] records an attribute, and [Span.RecordError] marks the span as failed. People
// who must not see the app's data read spans: record ids, counts and codes, never a form value,
// a name, an email address or any other personal data.
//
// Read more in the guide docs/guides/telemetry.md, which aicoded explain and the MCP tool howto
// print as guides/telemetry.
package telemetry
