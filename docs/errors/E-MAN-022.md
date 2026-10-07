# E-MAN-022: invalid ttl

`ttl` in the permission list (`aicoded.yaml`) is how long the app may live before it is reviewed, in days: a number from 1 to 3650, with no leading zero, followed by `d`. Months and years are written as days. Security sets it; leave it out unless the person or their administrator gives it.

```yaml
ttl: 6m     # wrong: not days
ttl: 180d   # right
```

See [the permission list](../guides/permission-list.md).

**Fix:** write a number of days from 1 to 3650 followed by `d`, such as `180d`.
