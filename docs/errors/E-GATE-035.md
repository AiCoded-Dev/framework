# E-GATE-035: what someone entered appeared in the app's log or telemetry

The simulated attacks (L7), which the delivery pipeline runs on the app it built, fill every text field of every form they post with a value of its own, such as `l7q4m2x8k1z0`. The personal-data step then searches, without regard to case, everything the app wrote while the attacks ran: its standard output and standard error, and every span it recorded, with its name, attributes, events and status. A value entered in a form was there. So the app logs or traces what people enter, which is personal data, such as names, email addresses or the text of a message; logs and spans reach more tools and more people than the app's data does, and they are kept. The problem is at the template of the page with the form, at line 1, and the message names the form, the field and where the value appeared, never the line itself. `aicoded check` refuses a log line, print or span that records a value from outside the app (E-LINT-008), but lint has limits, which its page lists; this step reads what the running app actually wrote. The path of each request, which the framework records in its span, is not searched. See [telemetry](../guides/telemetry.md) and [simulated attacks](../guides/simulated-attacks.md).

```go
slog.InfoContext(ctx, "suggestion saved", "text", s.Text) // wrong: what someone entered
slog.InfoContext(ctx, "suggestion saved", "id", s.ID)     // right: an id
```

**Fix:** log and record ids and counts, never what people enter: remove the value from the log line or span.
