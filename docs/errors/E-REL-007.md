# E-REL-007: calls between apps not run yet

The platform does not carry calls between apps yet. Rather than release an app whose calls would
all fail, it refuses one whose permission list has `services.calls` or `services.serves`, which
aicoded generate writes from the generated clients of other apps that the code uses and from the
functions in its `rpc/` folder. Nothing was released.

**Fix:** remove the calls to other apps and the functions in `rpc/`, run aicoded generate so that `services` is empty, commit, and publish again
