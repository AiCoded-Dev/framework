# E-REL-004: audience not allowed yet

For now the platform releases only apps that every person of the organisation may use: the
permission list must say `audience.internal: [everyone]`, with no other entry and no `external`
audience. For an organisation that signs in with its own company login, everyone is every
account that login signs in; for an organisation of invited people, its invited people. Apps for
a group or for people outside the company come later. Nothing was released.

```yaml
audience:
  internal: [everyone]
```

**Fix:** set `audience: {internal: [everyone]}` in aicoded.yaml, with no group and no external entry, commit, and publish again
