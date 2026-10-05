# E-GEN-042: reserved name in a template

A template expression, an `ssr:for` loop or an `<ssr:var>` uses a name that the generated code
declares or imports: `w` (the page writer), `s` (the page state), `render`, `web`, `form`,
`reactive`, `io`, `context`, `http`, `json`, `strings`, `sync`, `errors`, `slog`, or a name
starting with `_html`.
With such a name, a value could write to the page past the escapers. The package-level names
that the generated files of a page's package declare are reserved too, in expressions, loops,
`<ssr:var>` names and the types of `<ssr:var>` and `<ssr:call>`, since only the generated code
uses them: `routeKey`, `route`, `state`, `NewRoute`, `RouteData`, `RouteDataProvider`,
`ReactiveState`, `NewHandler`, `assets`, `assetsDir`, a name starting with `renderBlock_`, and
`Form<Name>Values`, such as `FormAddValues`, the type of a form's values. Inside `<ssr:form>`, `form`
is the form's values and may be used. The name of an `<ssr:var>` must also be a Go identifier,
because it becomes a Go variable. Attributes starting with `data-ssr-` are reserved too: the
generated code writes them to mark live values and bound inputs, so a hand-written one could
point the page at another value or make an element send writes. Types are written into code
that declares names of its own: the `type` of an `<ssr:var>` cannot use `s`, `r`, `ctx`, `conn`
or `msg`, and the `in` and `out` types of an `<ssr:call>` cannot use `s`, `p`, `r`, `ctx`, `in`,
`name` or `args`.

**Fix:** rename it; `w`, `s`, the names of the packages generated code imports, the names the page's generated files declare, such as `routeKey`, `state`, `RouteData` and `NewHandler`, and names starting with `_html` or `renderBlock_` are reserved, and so are `r`, `ctx`, `conn` and `msg` in a variable's type and `p`, `r`, `ctx`, `in`, `name` and `args` in a call's types
