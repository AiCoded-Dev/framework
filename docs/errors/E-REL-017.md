# E-REL-017: outside services of an old framework, or schedules

The platform calls an app's outside services only for an app built with framework v0.10.0 or
newer, whose `outbound.Client()` sends each request through the runner. It refuses to release or
preview an app whose permission list declares `egress` hosts and that was built with an older
framework: its runner would refuse to start it (E-RUN-014). The platform does not run schedules
yet either, so it refuses an app whose permission list declares `schedule` jobs, whatever its
framework, and the runner refuses to start one too. Nothing was released.

**Fix:** for `egress`, update aicoded.dev/framework to v0.10.0 or newer and publish again; for `schedule`, remove it from aicoded.yaml and the code that uses it until the platform runs schedules, run aicoded check, commit, and publish again
