# E-RPC-002: a malformed or misplaced //ssr:access line

A function of `rpc/` says who may call it with one line in its doc comment:
`//ssr:access caller=<app>[,<app>…] [role=<rule>] [apps=true]`. `caller=` is required and
names the calling apps by the `app:` line of their permission list (`aicoded.yaml`). `role=`
allows calls made on behalf of a viewer: the viewer needs one of the roles joined by `|`, such as
`role=editor|admin`, and `role=*` admits any viewer the calling app serves; roles are
lower-case names, and `*` stands alone. `apps=true` allows calls the calling app makes as
itself, with no viewer. At least one of `role=` and `apps=true` is required: without `role=`
no call on behalf of a viewer is allowed, and without `apps=true` no call without a viewer.
Attributes are separated by spaces; an unknown or repeated attribute, a value that breaks these
rules and a second `//ssr:access` line are refused. The line has an effect only on an exported
function, so it is refused on a method, an unexported function, a type, a var or a const. A line
written `// ssr:access`, with a space after `//`, is not read as one and is refused too.

**Fix:** write one line such as `//ssr:access caller=shop role=editor|admin` or `//ssr:access caller=shop,reports apps=true`, with no space after `//`, right above an exported function.
