# E-GEN-040: invalid `<ssr:call>`

A page call is a function of the page that the browser may call: the page's script runs
`ssr.call("name", args)` over the page's live connection, and the data provider answers in
`Call<Name>(ctx, r, in)`. The access rules and Guards of every page on the path run again before
each call, so a Guard sees a change, such as a record's new owner, at once. The roles are those
the viewer had when the live connection opened, so a role change takes effect when the page
reconnects, at the latest after 10 minutes. A call never runs `Data`. The arguments arrive as
JSON and must match the `in` type exactly, but they still come from the browser, so check them
in `Call<Name>`. The tag has no `name`, a name that is not a lower-case letter followed by
letters and digits (at most 63 characters), no `in` or no `out` type, an attribute other than
`name`, `in` and `out`, or a name another `<ssr:call>` of the template has, or it is inside an
element with `ssr:if`, `ssr:else-if`, `ssr:else` or `ssr:for`, or inside `<ssr:form>`: the page
can always make the call, whatever it shows. `in` and `out` are Go types, as for `<ssr:var>`;
`struct{}` stands for no arguments or no result. They cannot be a func or chan type, or a
pointer to one, because such a value cannot be sent as JSON.

**Fix:** write `<ssr:call name="lowerCamelName" in="InType" out="OutType"/>` once per name
