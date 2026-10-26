# E-DATA-001: data reaches mail

This is a note: it never stops `aicoded check` or a publish. A call of `mailer.Send` may send, in the message it gets, data from the app's database, its files, its inbox or a secret: a value scanned from the database, the name, contents or error of a stored file, a mail read from the inbox, or the value that `Reveal` of a `secrets.Value` returns. The same analysis as E-LINT-008 follows the data through strings, structs, slices, the app's own functions and the rest, so the note is at the call of `Send` that sends it, also inside a helper of the app, once per call, and the message names every source, such as `the app's database or a secret reaches mail`. Data from a form, the request or the viewer's identity is not noted: the viewer sees it already. Mail leaves the app and is kept by its recipients, so the change record lists each note for the people who approve what the app does. `aicoded check` shows the notes of each app, and `aicoded check --json` lists them in `notes`.

```go
mailer.Send(ctx, mailer.Message{IdempotencyKey: key, To: to, Text: "Invoice:\n" + body}) // body was read from the database
```

**Fix:** nothing, when the mail may carry this data to its recipients; otherwise send an id, a count or a link to the app instead
