# E-PUB-004: aicoded check found problems

Before a publish sends anything, `aicoded publish` runs `aicoded check --frozen --no-tests` on the app, as the delivery pipeline does again: every generated file must be current, and the app must build, vet and pass lint. It found the problems printed above, each with its code, `file:line` and fix, so nothing was sent. It checks as a released `aicoded` does, so a `replace` line is refused (E-LINT-011) even from an `aicoded` installed from a framework checkout. The tests do not run here; the delivery pipeline runs them. The `publish` tool of `aicoded mcp` lists the problems before this code.

**Fix:** fix every problem it lists, commit, then run `aicoded publish` again.
