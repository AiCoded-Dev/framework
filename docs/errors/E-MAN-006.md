# E-MAN-006: aicoded.yaml not found

Every app has a permission list, `aicoded.yaml`, in its directory. The command was run somewhere
else, or the file is missing.

**Fix:** run the command in the app's directory; `aicoded dev`, `aicoded check` and `aicoded describe` also run in a folder above several apps. A new app starts with at least `app: <name>`.
