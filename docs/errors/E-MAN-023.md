# E-MAN-023: an access entry is both guarded and shared

An entry of the `access` section of the permission list (`aicoded.yaml`) has both `guard: true`
and `shared: true`. `guard: true` says that a `Guard` decides who sees each record of the page,
and `shared: true` that everyone the access rules admit may see every record, so the entry
contradicts itself. `aicoded generate` writes the section from the pages' `<ssr:access>` and
never writes both, so the section was edited by hand. The problem is at the entry's path.

See [the permission list](../guides/permission-list.md).

**Fix:** run `aicoded generate` and do not edit the `access` section by hand
