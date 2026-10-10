# E-GEN-053: access rule that adds nothing

The page's `<ssr:access>` adds nothing to a rule above it on the path, so it does not limit who
may open the page the way it seems to. Every rule on the path from `pages/` to the page applies,
and a list admits anyone with one of its roles: `role="staff,people"` below `role="staff"`
admits every member of `staff`, with or without `people`. To require two roles, put them on
templates at different levels of the path, such as `role="staff"` on a layout and
`role="people"` on the page below it. `aicoded generate` refuses:

- a list of two or more different roles that includes every role of a rule above it, with or
  without `guard="true"` or `shared="true"`;
- `role="*"` without `guard="true"` or `shared="true"` below a rule that is not `*`: the rule
  above still decides who may open the page.

A single role repeated from a rule above, such as `role="staff"` below `role="staff"`, is
allowed, and so is `role="*"` below `role="*"`. So is a single role or `*` with
`guard="true"`, since the page's `Guard` still decides, or with `shared="true"`, which a page
with an id in its URL needs to say whose records it shows (E-GEN-054): `role="*" shared="true"`
keeps the rules above as they are. The error is at the page's `<ssr:access>` and names the
nearest rule above that it adds nothing to.

**Fix:** list only the roles this page adds; when it adds none, remove this `<ssr:access>`, or
keep one role with `guard="true"`
