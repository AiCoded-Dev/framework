# E-RUN-015: the secret is a key bound to an outside service

The app asked for the value of a secret that `egress` in its permission list (`aicoded.yaml`)
binds to a host. The runner adds such a key, in the header the binding names, to every request the
app sends to that host with `outbound.Client()`, so the key never reaches the app's memory, its
logs or its pages. Reading it is refused, and nothing was read.

```go
req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.partner.example/v1/items", http.NoBody)
if err != nil {
	return err
}
resp, err := outbound.Client().Do(req)
```

**Fix:** the runner adds it to every request to `<host>`: call the service with `outbound.Client()` and do not read the secret
