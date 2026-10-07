# E-MAN-017: invalid audience

`audience` in the permission list (`aicoded.yaml`) says who may use the app. `internal` lists people of the company: `group:<name>` for a group, with a name as in `owner`, and `everyone` for every employee. `external` lists outside people: `idp:<name>` for the users of a partner's identity provider, with a lower-case name of letters, digits and dashes, and `magic-link` for known outside users who sign in with a link sent to their email. An entry is in the wrong list, is not one of these forms, or is listed twice.

```yaml
audience:
  internal: [idp:partners]     # wrong: outside people under internal
  external: [idp:partners]     # right
```

See [the permission list](../guides/permission-list.md).

**Fix:** list people of the company as `group:<name>` or `everyone` under `internal`, and outside people as `idp:<name>` or `magic-link` under `external`, each once.
