# E-GATE-047: a function open to more callers than before

This is a note: it does not stop the publish. It is a reason its release *Needs approval*, and `aicoded publish` lists it under `Needs approval:` as `function <f> is open to more callers than in the version that runs now`. A function in the app's `rpc/` folder that the version running now also has is open to more callers: its `//ssr:access` line names an app in `caller=` that it did not name before, or a `role=` that admits viewers whom the rule did not admit before. Whatever the app type allows, such a change always needs an approver, since apps and people who could not reach its data now can. A new function is not more accessible: it had no one to compare with. For now the platform does not release an app that serves calls (E-REL-007). See [app types](../guides/app-types.md).

```go
// Contacts returns a person's contacts. front-desk can now call it, as people could before.
//
//ssr:access caller=people,front-desk role=staff
func Contacts(ctx context.Context, in ContactsIn) (ContactsOut, error)
```

**Fix:** an approver sees the function and who can now call it on the approval card; to stay Pre-approved, keep the function's caller= and role= as they were
