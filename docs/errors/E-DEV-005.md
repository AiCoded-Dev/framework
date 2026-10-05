# E-DEV-005: socket path too long

Unix socket paths are limited to about 100 bytes, and the directory `aicoded dev` got for the
app's sockets is longer. That directory is `XDG_RUNTIME_DIR` when it is set, otherwise `TMPDIR`
(`/tmp` when neither is set).

**Fix:** set the variable the error names (`XDG_RUNTIME_DIR` if it is set, otherwise `TMPDIR`) to
a short directory such as `/tmp` and run again.
