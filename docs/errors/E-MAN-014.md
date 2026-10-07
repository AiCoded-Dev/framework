# E-MAN-014: unknown key in aicoded.yaml

A key in the permission list (`aicoded.yaml`) is not one the permission list has, so it would say nothing to anyone who reads the list. Keys are checked at the top level, in `audience`, in each entry of `data` and `schedule`, in `email`, and in the sections `access` and `services`, which `aicoded generate` writes. The message names the key and where it is, and its fix lists the keys allowed there. The top-level keys are `app`, `class`, `owner`, `audience`, `data`, `access`, `services`, `egress`, `email`, `schedule`, `settings`, `secrets`, `modules`, `size`, `resources` and `ttl`.

```yaml
owners: group:hr-leads   # wrong: there is no key owners
owner: group:hr-leads    # right
```

See [the permission list](../guides/permission-list.md) for every key.

**Fix:** remove it, or correct its name to one of the keys the fix lists; in `access` and `services`, remove it: `aicoded generate` writes these sections from the code, so never edit them.
