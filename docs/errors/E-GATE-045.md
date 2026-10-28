# E-GATE-045: data reaching mail in a new way

This is a note: it does not stop the publish. It is a reason its release *Needs approval*, and `aicoded publish` lists it under `Needs approval:`. The data checks (L8) found data from the app's database, its files, its inbox or a secret that its mail may send (E-DATA-001), in a way the version that runs now did not have. Whatever the app type allows, data that may leave the company by mail in a new way always needs an approver. The note's message is that of the E-DATA-001 note it comes from. See [app types](../guides/app-types.md).

```go
mailer.Send(ctx, mailer.Message{IdempotencyKey: key, To: to, Text: "Invoice:\n" + body}) // body was read from the database
```

**Fix:** an approver sees what the mail may now carry on the approval card; to stay Pre-approved, send an id, a count or a link to the app instead of the data
