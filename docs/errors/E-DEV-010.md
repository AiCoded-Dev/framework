# E-DEV-010: two apps with one name

`aicoded dev`, `aicoded check` and `aicoded describe` work on every app below the folder they run in, and `aicoded generate` and `aicoded rpc add` look at the apps in the folders next to the app's own. They know each app by the `app:` name in its `aicoded.yaml`, which names the app's address, its calls and its local values in `dev.yaml`. Two apps in the workspace have the same name, often because one folder was copied from the other. The message names both folders. When such an app appears while `aicoded dev` runs, only that app fails with this error, and the app that was there first keeps running.

**Fix:** give each app its own name in the `app:` line of its `aicoded.yaml`, or start `aicoded dev` in a folder that holds only one of them.
