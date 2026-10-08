# E-GATE-018: a secret in the code

gitleaks, which the delivery pipeline runs over the commit and its history, found a secret, such as a password, an API key, a private key or a token, in a file of the commit that was published, at the line of the problem. The message names gitleaks' rule, such as `generic-api-key`, and never the secret itself. Anyone who can read the repository or a copy of it can use the secret, and an app never holds credentials in its code: the permission list declares each secret in `secrets:`, and the app reads it at run time with `secrets.Get`. Treat the secret as known to others, so its owner revokes it and issues a new one. Removing it in a new commit leaves it in the history, where E-GATE-019 finds it. A `.gitleaksignore`, a `.gitleaks.toml` or a `gitleaks:allow` comment in the repository changes nothing. See [settings and secrets](../guides/settings-and-secrets.md).

```go
const partnerKey = "the key itself" // wrong: the key is in the code and in its history

key, err := secrets.Get(ctx, "partner_api_key") // right: declared in secrets: of aicoded.yaml
```

**Fix:** remove the secret from the file, keep it in secrets: and read it with secrets.Get, ask its owner to revoke it, commit, and publish again.
