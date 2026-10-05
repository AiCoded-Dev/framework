# E-MAN-012: email not declared

The app sends or reads mail, but its permission list (`aicoded.yaml`) has no `email` section. The section names the one address the app sends from and the domains it may send to, so security sees where its mail can go.

**Fix:** add `email:` with `from:` and `to_domains:` to `aicoded.yaml`.
