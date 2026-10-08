# E-GATE-011: aicoded.yaml names another app

The delivery pipeline reads the permission list, `aicoded.yaml`, at the top of the commit before the checks that need it, and refuses the commit when its `app:` line names another app than the one the publish was for, or none. The problem is at the `app:` line when there is one. The permission list states what one app may do, so it must be that app's own. `aicoded publish` takes the app's name from that line, so this comes when the commit's `aicoded.yaml` is not the one `aicoded publish` read, or when the publish was not made by `aicoded publish`. A permission list that is missing or cannot be read is E-GATE-027.

```yaml
app: rooms-v2   # wrong: the publish was for rooms
app: rooms      # right
```

**Fix:** run aicoded publish in the app's folder, on a commit whose aicoded.yaml names the app.
