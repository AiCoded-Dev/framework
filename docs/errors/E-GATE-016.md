# E-GATE-016: an HTTP response body is not closed

bodyclose, which the delivery pipeline runs through golangci-lint, found an HTTP response whose body the code never closes. Each such response keeps its connection open, so the app runs out of connections and memory under load. App code cannot use the HTTP client of `net/http` (E-LINT-009), so this is mostly in a test, which the checks read too. The problem is at the line that gets the response. `aicoded check` does not run bodyclose, so only a publish shows it.

```go
resp, err := client.Do(req) // wrong: resp.Body is never closed
require.NoError(t, err)

resp, err := client.Do(req) // right
require.NoError(t, err)
defer resp.Body.Close()
```

**Fix:** close the response body with defer resp.Body.Close() after checking the error.
