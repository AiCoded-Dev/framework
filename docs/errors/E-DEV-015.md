# E-DEV-015: app renamed while aicoded dev runs

`aicoded dev` knows each app by the `app:` name in its permission list, `aicoded.yaml`, as it read it at start-up. That name gives the app its address, the calls other apps make to it and its local values in `dev.yaml`. The `app:` line of the app the message names has changed since, so `aicoded dev` stops that app rather than run it under a name it does not know. The other apps keep running. Once `aicoded dev` sees the change to `aicoded.yaml`, it removes the app under the old name and starts it under the new one, with the values of the new name in `dev.yaml`.

**Fix:** change `app:` in the permission list back to the old name, or wait for `aicoded dev` to restart the app under its new name.
