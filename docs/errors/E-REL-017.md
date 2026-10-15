# E-REL-017: outside services and schedules not run yet

The platform does not run an app's outside services or its schedule yet: no outbound call leaves
its server and no scheduled job fires. Rather than release an app whose outbound calls would fail
and whose jobs would never run, it refuses one whose permission list declares `egress` hosts or
`schedule` jobs, and the runner refuses to start one too. Nothing was released.

**Fix:** remove `egress` and `schedule` from aicoded.yaml until the platform runs them, and the code that uses them, run aicoded check, commit, and publish again
