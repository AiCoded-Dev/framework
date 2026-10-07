# E-MAN-016: invalid owner

`owner` in the permission list (`aicoded.yaml`) names the group of the company that owns the app, written `group:<name>`. The group's name is lower-case letters, digits, dots, dashes and underscores, starts and ends with a letter or digit, and is at most 64 characters. A single person, such as `user:a@acme.example`, cannot own an app.

```yaml
owner: hr-leads         # wrong: no group: in front
owner: group:hr-leads   # right
```

See [the permission list](../guides/permission-list.md).

**Fix:** write the group that owns the app as `group:<name>`, such as `group:hr-leads`: lower-case letters, digits, dots, dashes and underscores.
