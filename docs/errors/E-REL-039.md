# E-REL-039: held for its owner's preview

A note on a publish that passed and is held for its owner's preview: it does not go live and has
no approval card until its owner asks for its release. Every publish is held so, unless it was
published with `aicoded publish --release`, or the `publish` tool with `release` set. The note is
for an `aicoded` older than v0.13.0, which does not know the held state; a newer one shows
`Held for preview, not live: its owner asks for its release with aicoded release <id>.` instead,
and leaves the note out. The note does not stop the publish and is not part of its change record.

```sh
aicoded preview <id>    # try the publish in the app's preview
aicoded release <id>    # then ask for its release
```

See [publishing](../guides/publishing.md).

**Fix:** its owner asks for its release: `aicoded release <id>` (aicoded v0.13.0 or newer) or the Release button on the app's Production page
