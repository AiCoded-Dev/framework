# E-DEV-011: gateway port in use

`aicoded dev` serves every app of the workspace through one gateway on 127.0.0.1, at `http://<app>.localhost:<port>`. Another program already listens on that port, often another `aicoded dev` or a web server, so `aicoded dev` stops before it builds anything. The message names the port.

**Fix:** stop the program that uses the port, or set `port:` under `workspaces.<workspace folder>` in `dev.yaml` to a free port.
