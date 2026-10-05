# E-DEV-003: no local values

The app declares settings or secrets in `aicoded.yaml`, but `dev.yaml` has no local value for
them, so `aicoded dev` cannot start it.

**Fix:** add the listed names under `workspaces.<directory>.apps.<app>.settings` or `.secrets`
in `dev.yaml` (see E-DEV-002 for the format). Use test values, never production secrets.
