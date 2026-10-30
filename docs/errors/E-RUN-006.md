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

At an app's preview, `<app>.<organisation>.preview.aicoded.cloud`, the audience does not apply.
The platform shows this page, titled "You cannot open this preview", to everyone of the
organisation but the app's owner, the up to five people the owner picked in the console, and,
while an approval card of the app waits, whoever may approve it. It checks again every 15
minutes, so a person the owner removed, or an approver once the card is approved or declined, sees
this page too.

See [app types](../guides/app-types.md) and [publishing](../guides/publishing.md).

**Fix:** if you should use the app, ask your security admin to add you to a group the app's audience names; the app's owner can add a mapped group to audience.internal and publish again. At a preview: if you should see the preview, ask the app's owner to add you to its viewers in the console
