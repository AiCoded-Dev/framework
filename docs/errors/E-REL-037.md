# E-REL-037: this publish is not held for its owner's preview

`aicoded release <id>`, the `release` tool or the **Release** button of the console asked the
platform to release a publish that it does not release on that request. The message says which
case it is:

- `publish <id> is not held for its owner's preview: it was published with --release, so its
  release was asked for at once`: the publish was sent with `aicoded publish --release`, or the
  `publish` tool with `release` set, and it went live, waits for an approver or was not released.
- `publish <id> is not held for its owner's preview: it has not passed its checks`: it is still
  being checked, or it failed.
- `publish <id> is held for its owner's preview, and only its app's owner asks for its release`:
  the platform's operator asked. The fix is then: the app's owner asks for its release, with
  aicoded release or the Release button of the console.
- `publish <id> was held for its owner's preview and its release was never asked for`: the
  platform's operator asked for a publish that was held and has since been superseded or refused.
  The fix is then: the app's owner asks for the release of the app's newest publish, with aicoded
  release or the Release button of the console.

Anyone else but the app's owner gets E-PUB-005 from `aicoded`, or 403 in the console.

Every other publish that passed is held until its owner asks for its release. Asking again for the
release of a held publish whose release was asked for already is not refused: it shows the publish
as it is. Nothing was released.

```sh
aicoded status <id>     # "Held for preview" can be released
aicoded release <id>
```

See [publishing](../guides/publishing.md).

**Fix:** release only a held publish that passed its checks: aicoded status shows it as held
