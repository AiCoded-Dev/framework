# E-MAN-015: invalid class

`class` in the permission list (`aicoded.yaml`) names the app type the app belongs to, which security defines. It is the app type's name: lower-case letters, digits and dashes, starting with a letter, at most 63 characters. Leave it out unless the person or their administrator gives it.

```yaml
class: Internal_Tool   # wrong: capitals and an underscore
class: internal-tool   # right
```

See [the permission list](../guides/permission-list.md).

**Fix:** use the app type's name: lower-case letters, digits and dashes, such as `internal-tool`.
