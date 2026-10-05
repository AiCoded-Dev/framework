# E-DEV-001: dev.yaml is readable by other users

`dev.yaml` holds local values of secrets, so `aicoded dev` refuses to read it while other users
can.

**Fix:** `chmod 600 <path to dev.yaml>`.
