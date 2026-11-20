# E-REL-038: this publish was superseded by a newer publish of the app

`aicoded release <id>`, the `release` tool or the **Release** button of the console asked the
platform to release a publish held for its owner's preview, and a newer publish of the app has
since been decided on: `publish <id> was superseded by a newer publish of the app, and only the
app's newest publish is released`. The newer publish supersedes the held one, as it supersedes one
that is going live or waiting for an approver, so that an older version never replaces a newer
one. Nothing was released.

```sh
aicoded status <new id>   # the newest publish, held for its preview
aicoded release <new id>
```

See [publishing](../guides/publishing.md).

**Fix:** release the app's newest publish
