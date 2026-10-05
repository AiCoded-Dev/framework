# E-DEV-017: control socket path too long

`aicoded dev` serves a control socket, `control.sock`, in the workspace's private state folder under `~/.local/state/aicoded` in your home folder. Unix socket paths are limited to about 100 bytes, and the path of this one is longer, because the path of your home folder is long.

**Fix:** use a home folder with a shorter path.
