# E-GATE-027: aicoded.yaml is missing or not readable

The delivery pipeline reads the permission list, `aicoded.yaml`, at the top of the commit before the checks that need it, and refuses the commit when the file is missing, is larger than 64 KiB, is not a plain file, such as a symbolic link, is not UTF-8 text, does not parse as YAML, or uses an anchor, alias or merge key (`&name`, `*name` or `<<`), as `aicoded check` refuses it too (E-MAN-003). Without it, the platform cannot tell what the app may do. The message says which, and its fix is the one for that case. `aicoded publish` refuses a permission list that does not load, so this comes when the commit's `aicoded.yaml` is not the one `aicoded publish` read, such as one that `.gitignore` names, or when the publish was not made by `aicoded publish`.

```sh
git check-ignore -v aicoded.yaml   # names the .gitignore line that leaves the file out; remove that line
git add .gitignore aicoded.yaml
git commit -m "Add the permission list"
```

**Fix:** the one for the case the message names:

- missing: add aicoded.yaml with `app: <the app's name>` to the app's folder, commit, and publish again;
- larger than 64 KiB: make aicoded.yaml smaller than 64 KiB;
- not a plain file: make aicoded.yaml a plain file, not a link;
- not UTF-8: save aicoded.yaml as UTF-8;
- not YAML, or with an anchor, alias or merge key: fix aicoded.yaml so aicoded check passes, commit, and publish again.
