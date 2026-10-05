# E-RUN-001: no runner

The app was started without a runner. `AICODED_RUNNER_DIR` tells the app where the runner's
sockets are, and only a runner sets it. An app cannot serve anything on its own: it has no
network, no database and no viewers without its runner. To run the app by hand, for example
under a debugger, start `aicoded dev --manual <app>`: it keeps the app's runner up and prints
the command that starts the app with `AICODED_RUNNER_DIR` set.

**Fix:** start the app with `aicoded dev`, or run it by hand with the command `aicoded dev --manual <app>` prints.
