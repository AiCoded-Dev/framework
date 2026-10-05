# E-MAN-002: secret not declared

The app asked for a secret that `aicoded.yaml` does not declare. The runner hands out only
declared secrets.

**Fix:** add the name under `secrets:` in `aicoded.yaml`, and give it a value (locally in
`dev.yaml`, on the platform in the console).
