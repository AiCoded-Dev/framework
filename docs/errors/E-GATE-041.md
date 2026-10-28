# E-GATE-041: mail to a domain the app type does not allow

This is a note: it does not stop the publish. It is a reason its release *Needs approval*, and `aicoded publish` lists it under `Needs approval:`. The app's permission list names, in `email.to_domains`, a domain that its app type does not allow mail to. The Internal tool allows only the company's own domains: those its people sign in with, and its verified mail domains. See [app types](../guides/app-types.md).

```yaml
email:
  from: rooms@acme.example
  to_domains: [acme.example, partner.example]   # partner.example is outside the company
```

**Fix:** an approver sees mail to the domain as new on the approval card; to stay Pre-approved, mail only domains the app type allows
