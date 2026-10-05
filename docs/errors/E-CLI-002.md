# E-CLI-002: app name not valid or folder taken

`aicoded init <name>` and the `app_create` tool of `aicoded mcp` create a new app in a new folder named after the app, in this folder: the current folder for `aicoded init`, and the workspace folder for `aicoded mcp`. The name is also the app's name in its permission list, `aicoded.yaml`, and part of its address in `aicoded dev`, so it must be 2 to 63 lowercase letters, digits and dashes, starting with a letter and ending with a letter or digit. The name given is not such a name, or a file or folder of that name is already there. Neither writes into an existing folder.

**Fix:** choose a name of lowercase letters, digits and dashes that no folder here has.
