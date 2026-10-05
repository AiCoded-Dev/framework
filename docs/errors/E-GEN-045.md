# E-GEN-045: invalid `<ssr:var>` or `<ssr:call>` type

The `type` of an `<ssr:var>` and the `in` and `out` types of an `<ssr:call>` are written into
the page's generated Go code, so each must be a type that code can name: a type declared in a
`.go` file in the page's folder, such as `Note`, or a predeclared one, such as `string`, possibly
with type arguments, or a pointer, slice, array, map or func of such types. An `<ssr:var>` may
also name an exported type of package `web`, which the generated file imports, such as
`type="web.SafeHTML"` for markup that `{{$ }}` writes. An `<ssr:call>` may use `struct{}` for a
call that takes or returns nothing, but no `web` type, because its values travel as JSON. A type
from any other package, such as `time.Time`, cannot be named there, because the generated file
imports only the framework. Anything else, such as a function call or a `struct{…}` literal with
fields, is not a type the generator writes.

**Fix:** use a type name, or a pointer, slice, array, map or func of them; declare other types
in a `.go` file next to the template
