# E-CLI-007: the credentials file can be read by others

`aicoded login` keeps your sign-in to the platform, which works like a password, in `credentials.json` in your user configuration folder: `~/.config/aicoded/credentials.json` on Linux (`$XDG_CONFIG_HOME/aicoded/credentials.json` if that is set) and `~/Library/Application Support/aicoded/credentials.json` on macOS. Only you may read the file, so its mode must be 600, and only you may open its folder, so its mode must be 700. `aicoded` refuses to use the file, or to write it, while others can. The message names the file or the folder and its mode.

**Fix:** run `chmod 600` on the file or `chmod 700` on the folder the message names. When others could read the file, also run `aicoded logout` and `aicoded login`, so that the sign-in they may have read is revoked.
