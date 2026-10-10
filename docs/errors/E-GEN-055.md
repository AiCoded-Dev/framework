# E-GEN-055: `shared="true"` where it means nothing

`shared="true"` on `<ssr:access>` says that everyone the access rules admit may see every record
of a page with an id in its URL, and of the pages below it. `aicoded generate` refuses it where
it cannot mean that:

- next to `guard="true"` on the same `<ssr:access>`: either the page's `Guard` decides who sees
  each record, or everyone the rules admit sees every record;
- on a template whose URL has no parameter: such a page does not show one record of many;
- below a template that declares `guard="true"`: that `Guard` runs for every page below it and
  still decides who sees each record.

`guard="true"` below `shared="true"` is allowed, because it narrows who sees the records of the
pages below it. The error is at the template's `<ssr:access>`; below a `Guard`, the message
names the route that declares it. See [access](../guides/access.md).

**Fix:** the one for the case the message names:

- both on one `<ssr:access>`: keep one: `guard="true"` when the Guard decides who sees each
  record, or `shared="true"`;
- no parameter in the URL: remove `shared="true"`: only a page with an id in its URL shows one
  record of many;
- below a Guard: remove `shared="true"`: the Guard above still decides who sees each record.
