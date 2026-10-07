# E-PUB-003: --app names another app

`aicoded publish --app <name>` publishes only when the app in the folder has that name, so that a script or an AI assistant never publishes the wrong app. The `app:` line of the folder's `aicoded.yaml` names another app, which the message quotes. Nothing was sent.

**Fix:** leave `--app` out, or run `aicoded publish` in the folder of the app it names.
