# E-REL-029: this publish cannot be previewed

`aicoded preview <id>`, or the `preview_publish` tool, asked the platform to run a publish as its
app's preview, and the publish has not passed its checks: it is still being checked, a check
that stops a publish failed, it was refused or ended in error, or an approver declined its
approval card. Only a publish that passed every such check runs as a preview. The app type's
limits do not apply to a preview, so a publish that *Needs approval* can be previewed while its
approval card waits. Nothing was started, and a preview that runs keeps its version.

```sh
aicoded status <id>     # passed, with no Declined line, can be previewed
aicoded preview <id>
```

See [publishing](../guides/publishing.md).

**Fix:** preview a publish that passed its checks; publish again to get one
