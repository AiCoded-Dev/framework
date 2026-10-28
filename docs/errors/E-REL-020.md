# E-REL-020: a group the group map does not have

The app names a group, in `audience.internal` as `group:<name>`, or a role, in an access rule as `role="<name>"`, that the organisation's group map does not have. The group map is how the platform knows who is in a group: each name stands for a group of the company login or, in an organisation of invited people, for a list of them, and a role `<name>` is membership of `group:<name>`. A name it does not have would let in no one, or someone security never meant, so the publish fails, with this problem in its step `decision`, and a release is refused the same way while the name is missing. `everyone` needs no name. The message lists the names the group map has. See [app types](../guides/app-types.md).

```yaml
audience:
  internal: [group:hotel-staff]    # hotel-staff must be a name of the group map
```

**Fix:** ask your security admin to map the group, or use a mapped group that the message lists, commit, and publish again
