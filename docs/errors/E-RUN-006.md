# E-RUN-006: you are not in this app's audience

You signed in with the company login, but the app's permission list does not name you among the
people who may use it: you are not in a group that its `audience.internal` names, through the
organisation's group map. The runner shows this page, with status 403, and passes nothing to the
app. It checks again every 15 minutes, so someone who leaves a group that the audience names is
signed out of the app and sees this page too.

```yaml
audience:
  internal: [group:hotel-staff]    # only people the group map puts in hotel-staff
```

See [app types](../guides/app-types.md).

**Fix:** if you should use the app, ask your security admin to add you to a group the app's audience names; the app's owner can add a mapped group to audience.internal and publish again
