# E-GEN-031: invalid `<ssr:access>`

The template's `<ssr:access>` has no `role`, a role that is not a lower-case name, `*` together
with other roles, a `guard` or `shared` other than `"true"` or another attribute, or the template
has more than one `<ssr:access>`. The syntax is `<ssr:access role="a,b" guard="true"/>` or
`<ssr:access role="a,b" shared="true"/>`. `role` lists the roles, separated by commas, and the
viewer needs one of them. A role name is lower-case letters, digits and `-`, starting with a
letter, at most 63 characters long. `*` stands alone and admits every viewer the company login
lets into the app. `guard="true"` is optional and declares the route's `Guard` method, which runs
after the role checks. `shared="true"` is optional and says that everyone the rules admit may see
every record of a page with an id in its URL (E-GEN-054). Rules on a path are combined: the
viewer needs a role from every rule on the path from `pages/` to the page. The rule always
applies, so it must not be nested inside an element with `ssr:if`, `ssr:else-if`, `ssr:else` or
`ssr:for`, or inside `<ssr:form>`.

**Fix:** write one `<ssr:access role="a,b"/>` per template; roles are lower-case names or `*`
