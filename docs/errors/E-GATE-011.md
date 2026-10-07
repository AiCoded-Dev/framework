# E-GATE-011: aicoded.yaml is missing, not readable or names another app

The delivery pipeline reads the permission list, `aicoded.yaml`, at the top of the commit before any other step, and refuses the commit when the file is missing, is not a plain file, is larger than 64 KiB, is not UTF-8 text, does not parse as YAML, or names in its `app:` line another app than the one the publish was for, or none. The message says which, at the `app:` line when there is one. `aicoded publish` takes the app's name from that line and refuses a permission list that does not load, so this comes when the commit's `aicoded.yaml` is not the one `aicoded publish` read, such as one never committed or changed since, or when the publish was not made by `aicoded publish`.

**Fix:** commit the app's `aicoded.yaml` at the top of its repository, as plain UTF-8 YAML of at most 64 KiB whose `app:` line names the app, then run `aicoded publish` in the app's folder.
