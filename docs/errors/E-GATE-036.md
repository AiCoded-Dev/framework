# E-GATE-036: the app did not start, or stopped, during the attacks

For the simulated attacks (L7), the delivery pipeline starts the program it built under a runner, in a sandbox with no network, in the environment `preview`: with a new, empty database when the permission list declares `sqldb`, empty file stores, mail that is caught and never sent, each setting set to `l7-` and its name, such as `l7-report_day`, and each secret set to a random value. The app did not report that it was ready within 30 seconds, it exited before that, or it exited while the attacks ran. The attacks need the app running, so the publish fails. The problem is at `main.go:1`, and the message holds the end of the app's output. Look there for what stopped it, such as an `OnStart` that fails on an empty database, or one that refuses the value of a setting: keep the value `OnStart` reads, and check it where the app uses it. An app that runs out of memory is E-GATE-012 instead. See [simulated attacks](../guides/simulated-attacks.md).

```text
  trips: main.go:1: E-GATE-036: the app exited before it was ready; its output ends: ... report_day is not a weekday ...
    fix: start the app with aicoded dev, fix the error its log shows, and publish again
    docs: https://aicoded.dev/docs/errors/E-GATE-036
```

**Fix:** start the app with aicoded dev, fix the error its log shows, and publish again.
