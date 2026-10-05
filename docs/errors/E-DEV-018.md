# E-DEV-018: the dev UI's token file is not valid

`aicoded dev` keeps the access token of its dev UI in `ui-token`, in the workspace's private state folder under `~/.local/state/aicoded`, and prints a login link that holds it. That file must be a regular file of mode 0600 that holds 64 lowercase hex characters, and `aicoded dev` makes it when it is missing. The file at the path the message names is something else: a folder, a symbolic link, a file that other users can read, or a file with other content.

**Fix:** delete the file the message names; `aicoded dev` makes a new token at its next start.
