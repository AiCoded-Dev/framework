# E-GATE-037: the app wrote more log or telemetry than the checks read

The personal-data step of the delivery pipeline reads everything the app wrote while the simulated attacks (L7) ran: its standard output and standard error, and every span it recorded. It reads at most 32 MiB of output and 100,000 spans. The app wrote more, or dropped spans itself, so part of what it recorded went unread, and a value someone entered could be in that part. The step fails rather than pass what it could not read. The problem is at `main.go:1`, and the message says which limit the app passed. The attacks send a few thousand requests at most, so an app comes near these limits only when it records much for each request, such as a log line for every row it reads or a span for every value. See [telemetry](../guides/telemetry.md) and [simulated attacks](../guides/simulated-attacks.md).

```go
for _, room := range rooms {
	slog.InfoContext(ctx, "room listed", "id", room.ID) // wrong: a line for every row
}

slog.InfoContext(ctx, "rooms listed", "count", len(rooms)) // right: one line per action
```

**Fix:** log less: one line per action, with ids and counts.
