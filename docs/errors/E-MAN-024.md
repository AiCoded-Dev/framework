# E-MAN-024: not allowed

The permission list (`aicoded.yaml`) asks for something no app may do, whatever its app type: the release would be *Not allowed*, so `aicoded check`, the delivery pipeline and the runner refuse it. One of these is true:

- `audience` lists no one: it is left out, or neither `internal` nor `external` has an entry. Every app is used by someone who signs in, and no app is open to anyone.
- A data source has the class `health`, `hr` or `card`: health, HR and payment card data stay out of apps.

```yaml
audience:
  internal: [everyone]             # every employee; or [group:<name>] for a group
data:
  - source: sqldb
    classes: [internal]            # never health, hr or card
```

See [the permission list](../guides/permission-list.md).

**Fix:** list who uses the app under `audience`, such as `internal: [everyone]`; remove the class `health`, `hr` or `card`, and the data it names, from the app.
