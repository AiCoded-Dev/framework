# E-DEV-014: no app with that name

A command or an `aicoded mcp` tool named an app that the workspace does not have. The workspace is every folder with an `aicoded.yaml` below the folder where `aicoded dev`, `check`, `describe` or `mcp` runs. An app is named by the `app:` line of its `aicoded.yaml`, never by a folder or a path, so a name with a slash or a capital letter is refused the same way.

**Fix:** name an app from the app: line of an aicoded.yaml in this workspace.
