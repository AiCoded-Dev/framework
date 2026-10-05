# E-DEV-013: aicoded dev of another version

`aicoded mcp` and `aicoded dev` talk to a running `aicoded dev` through its control socket, and both sides must speak the same version of that protocol. The running `aicoded dev` was started by an `aicoded` of another version, often one installed before an update.

**Fix:** stop the running `aicoded dev` and start it again with this `aicoded`.
