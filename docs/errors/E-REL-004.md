# E-REL-004: people outside the company not served yet

For now the platform releases only apps used by people of the organisation: those that
`audience.internal` names, as `everyone` or `group:<name>`. People outside the company, whom
`audience.external` names, such as `magic-link` or `idp:<name>`, cannot sign in to an app yet, so
the platform refuses a release whose permission list has an `external` entry. Nothing was
released.

```yaml
audience:
  internal: [group:hotel-staff]
  external: []
```

**Fix:** remove the entries of audience.external from aicoded.yaml, commit, and publish again
