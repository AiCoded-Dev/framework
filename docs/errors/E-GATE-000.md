# E-GATE-000: a check found something with no code of its own

One of the security checks of the delivery pipeline reported a finding that the platform has not given a code of its own, such as a rule that a newer version of a scanner added. The message names the check and its rule, and the problem is at the line the check points to. It stops the publish like any other problem, so that a finding is never lost for want of a code.

```text
  rooms: deps/rooms.go:31: E-GATE-000: typecheck: undefined: listRooms
    fix: read the message: it names the check and its rule; fix the code it points to, and publish again
    docs: https://aicoded.dev/docs/errors/E-GATE-000
```

See [publishing](../guides/publishing.md) for the checks and what each one runs.

**Fix:** read the message: it names the check and its rule; fix the code it points to, and publish again.
