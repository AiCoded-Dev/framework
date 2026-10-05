# E-MAN-001: setting not declared

The app read a setting that `aicoded.yaml` does not declare. Settings must be declared so the
permission list shows everything the app is configured with.

**Fix:** add the name under `settings:` in `aicoded.yaml`, and give it a value (locally in
`dev.yaml`, on the platform in the console).
