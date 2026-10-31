# Call an outside service

Call a service on the internet over HTTPS, with a key the runner adds so that the app never reads
it. The example asks a weather service for a city's weather, from a method of the app's `Deps`.

## Steps

1. Declare the service in the permission list. A service that needs no key is a host alone; one
   that needs a key binds a secret to the header the service reads it from:

   ```yaml
   egress:
     - host: api.weather.example
       secret: weather_api_key
       header: Authorization
       prefix: "Bearer "
   secrets: [weather_api_key]
   ```

   Ask the developer to put a test key in `dev.yaml`, under the app's `secrets`, and never write
   a key anywhere in the app. [The permission list](../guides/permission-list.md) has the rules
   for hosts and bindings.
2. Keep the client of `outbound.Client()` in an unexported field of `Deps`:

   <!-- code: outbound/example_test.go Deps -->
   ```go
   // Deps is what an app's pages share, as deps.Deps is in an app. It keeps the client in an
   // unexported field: aicoded check refuses a value that holds a client in an exported field when
   // it becomes an interface value, as each page's data provider does in the generated code
   // (E-LINT-009).
   type Deps struct {
   	weather *http.Client
   }
   ```

   <!-- code: outbound/example_test.go New -->
   ```go
   // New returns Deps with the client whose requests go through the runner.
   func New() *Deps {
   	return &Deps{weather: outbound.Client()}
   }
   ```

3. Call the service from a method of `Deps`, with a request made by `http.NewRequestWithContext`
   and the context of the request or job it serves, and send it with `Do`:

   <!-- code: outbound/example_test.go Deps.Today -->
   ```go
   // Today returns the weather service's summary of today in city. ctx is the context of the
   // request or job it serves. The service's host, api.weather.example, is in egress in the
   // permission list, and the runner adds the key that egress binds to it.
   func (d *Deps) Today(ctx context.Context, city string) (string, error) {
   	u := "https://api.weather.example/v1/today?city=" + url.QueryEscape(city)
   	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
   	if err != nil {
   		return "", err
   	}
   	resp, err := d.weather.Do(req)
   	if err != nil {
   		return "", err
   	}
   	defer func() { _ = resp.Body.Close() }()
   	if resp.StatusCode != http.StatusOK {
   		return "", fmt.Errorf("the weather service answered %s", resp.Status)
   	}
   	var today struct{ Summary string }
   	if err := json.NewDecoder(resp.Body).Decode(&today); err != nil {
   		return "", err
   	}
   	return today.Summary, nil
   }
   ```

   - The request sets no `Authorization` header: the runner adds it, and replaces one the app
     sets. `secrets.Get(ctx, "weather_api_key")` fails with E-RUN-015.
   - A page calls it from `Data` with the context `Data` gets, as `p.d.Today(ctx, city)`, and
     returns its error.
   - Escape every value that goes in the URL, as `url.QueryEscape` does here.

## Check it

- `aicoded check` refuses any other HTTP client, and any change to this one (E-LINT-009).
- `aicoded dev` sends the request to the real service with the key from `dev.yaml`. The dev UI's
  Traces page shows the page's request with a span `outbound` that names the host, the method and
  the status, and never the path, the headers or the key.
- A host missing from `egress` is E-OUT-001; the other refusals and the limits are in
  [outside services](../guides/outbound.md).
- On the platform, the new binding *Needs approval*: the approver sees which key goes to which
  outside service.

## See also

- [Outside services](../guides/outbound.md)
- [Settings and secrets](../guides/settings-and-secrets.md)
- [aicoded dev](../guides/dev.md)
