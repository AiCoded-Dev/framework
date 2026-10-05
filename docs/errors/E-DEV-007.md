# E-DEV-007: MySQL admin DSN not usable

The `mysql:` value in `dev.yaml` is not a valid DSN, or it points at a server that is not on this machine. `aicoded dev` talks to MySQL over a Unix socket or loopback TCP only, so no password or data crosses a network.

**Fix:** write `user:password@unix(/var/run/mysqld/mysqld.sock)/` or `user:password@tcp(127.0.0.1:3306)/`.
