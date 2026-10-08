# E-GATE-019: a secret in the history

gitleaks, which the delivery pipeline runs over the commit and its history, found a secret, such as a password, an API key, a private key or a token, in an earlier commit of the history the publish sent, though a later commit may have removed it. The problem is at the file and line of that commit, and the message names gitleaks' rule and the commit, never the secret itself. Git keeps every commit, so anyone who can read the repository, a copy of it or a bundle sent to the platform can still read the secret. A bundle holding the secret stays on the platform until an administrator purges it. A `.gitleaksignore`, a `.gitleaks.toml` or a `gitleaks:allow` comment in the repository changes nothing.

```text
5d6e7f8 Read the partner key with secrets.Get   removes the key from deps/partner.go
1a2b3c4 Call the partner's API                  adds deps/partner.go with the key: E-GATE-019
```

Once no commit of the history holds the secret, such as after a rebase that edits the commit that added it, or `git commit --amend` when that commit is the last one, the next publish sends the whole rewritten history, and its change record says that the history was rewritten. See [publishing](../guides/publishing.md).

**Fix:** ask the secret's owner to revoke it, rewrite the history so no commit holds it, and ask your administrator to purge the earlier bundles.
