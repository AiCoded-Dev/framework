# E-DEV-016: something else at the control socket's path

`aicoded dev` serves a control socket, `control.sock`, in the workspace's private state folder under `~/.local/state/aicoded`, so that `aicoded mcp` and a second `aicoded dev` can reach it. A file or folder that is not a socket is at that path. `aicoded dev` never removes it, since it did not make it, so it cannot start.

**Fix:** remove the file the message names if nothing else uses it, then start `aicoded dev` again.
