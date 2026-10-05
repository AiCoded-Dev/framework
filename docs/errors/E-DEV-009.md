# E-DEV-009: state folder readable by other users

`aicoded dev` keeps each workspace's files, such as the contents of its apps' file stores, in a private folder under `~/.local/state/aicoded`. That folder must be readable by you only.

**Fix:** run `chmod 700` on the folder the message names.
