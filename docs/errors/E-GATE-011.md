# E-GATE-011: aicoded.yaml names another app

The `app:` line of the commit's `aicoded.yaml` names another app than the one the publish was for. `aicoded publish` takes the app's name from that line, so the two differ only when the publish was not made from that commit's `aicoded.yaml`.

**Fix:** run `aicoded publish` in the app's folder, on a commit whose `aicoded.yaml` names the app.
