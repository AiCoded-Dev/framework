# Outside services

`outbound.Client()` returns the `*http.Client` an app calls outside services with. Every request
goes to the runner, which sends it only to a host that `egress` in the permission list names,
over HTTPS, adds the key bound to that host, and bounds its size and time. The key never reaches
the app's code.

## Declaring them

`egress` lists each outside service by host. An entry is the host alone, or the host with a
secret that the runner adds to every request to it:

```yaml
egress:
  - api.weather.example            # a host alone
  - host: api.partner.example      # a host with a key
    secret: partner_api_key        # a secret declared in secrets
    header: Authorization          # the header the key goes in
    prefix: "Bearer "              # text before the key, if the service wants one
secrets: [partner_api_key]
```

[The permission list](permission-list.md) has the rules for hosts (E-MAN-018) and bindings
(E-MAN-025). In short: a lower-case host name with no scheme, port, path, wildcard or IP address,
never one of the platform's own; each host once; a binding names a secret that `secrets` declares
and a header such as `Authorization` or `X-Api-Key`.

## Calling a service

Keep the client in an unexported field of `Deps`, and call the service from a method of `Deps`
with the context of the request or job it serves:

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

- Build each request with `http.NewRequestWithContext` and send it with the client's `Do`. A
  request without the app's context, such as one made with `context.Background()`, fails with
  E-RUN-004.
- Use the client as `outbound.Client()` returns it. `aicoded check` refuses any other HTTP client,
  such as `http.Get`, `http.DefaultClient` or an `http.Client{}` of the app's own, and any change
  to this one: its fields, its `Get`, `Head`, `Post` and `PostForm` methods, which send without
  the request's context, or turning it into an interface value (E-LINT-009). Hold it in a
  variable, a parameter or an unexported field; a value that holds it in an exported field may
  not become an interface value, which the generated code does with each page's `DP`.
- A refusal comes back as the error of `Do`, which wraps the coded error: its message holds the
  code, such as E-OUT-001, the reason and the fix. Return it; a page shows the viewer "Something
  went wrong.".

## The key never reaches the code

- The runner reads a bound secret itself and puts the prefix and the secret's value in the
  binding's header of every request to the host. It first removes every header of that name the
  app set, in any case, so only the runner's value goes out.
- `secrets.Get` of a bound secret fails with E-RUN-015. Read a secret yourself only when it is
  not a key for a service in `egress`, such as a key to sign data with.
- The runner never puts the key in an error, a log line or a span. What the service sends back
  reaches the app as it came: a service that echoes the request's headers echoes the key too.
- A new or changed binding always *Needs approval*, whatever the app type allows (E-GATE-048): the
  approver sees which key goes to which outside service on the approval card.

## Limits

| What | Limit | Code |
|---|---|---|
| Scheme and port | `https` to port 443; no user name or password in the URL | E-OUT-002 |
| Host | one that `egress` lists, by its lower-case name, never an IP address | E-OUT-001 |
| Address | public only: the platform's own hosts, private networks, this computer and cloud metadata are never reachable, whatever the host resolves to | E-OUT-003 |
| Request body | 10 MiB | E-OUT-004 |
| Response body | 10 MiB | E-OUT-005 |
| Time | 30 s for the whole request, connecting and reading the body included | E-OUT-006 |
| At once | 16 requests under way per instance of the app; the 17th is refused at once | E-OUT-007 |
| Headers | names of letters, digits and `-` and the other token characters; values without line breaks | E-OUT-009 |

- The runner resolves the host each time it connects, and refuses the request when any address
  of the answer is one no app may reach.
- It sends `Host`, `Content-Length` and the connection's own headers, such as `Connection` and
  `Transfer-Encoding`, itself, and drops those the app set.
- It follows no redirect: a 3xx response comes back with its `Location`. Send a new request to it
  when its host is in `egress`.
- A service it cannot reach, by name, connection or TLS certificate, is E-OUT-008. The reason is
  not passed on.
- The client itself gives up after 35 s, only when the runner does not answer.

## What the trace shows

The runner records a span named `outbound` for every request, in the trace of the request or job
that sent it: the host (`server.address`), the method, the status, the bytes sent and received,
the duration, and the code of a refusal (`aicoded.outbound.refused`). It never records the path,
the query, the headers or the bodies, which can hold personal data or keys.

## In aicoded dev

`aicoded dev` sends the requests to the real service on the internet, with the same checks as the
platform. A bound secret's value comes from `dev.yaml`, as for any secret: use a test key, never a
production one. A service on your computer or your network is unreachable here too (E-OUT-003).
The dev UI's Traces page shows each `outbound` span. See [aicoded dev](dev.md).

## On the platform

- A new host in `egress` is *Pre-approved* when the app type allows it, and *Needs approval*
  otherwise (E-GATE-042); a new or changed key binding always *Needs approval* (E-GATE-048). See
  [app types](app-types.md).
- An app with `egress` needs framework v0.10.0 or newer: the platform refuses to release an older
  one (E-REL-017), and the runner refuses to start it (E-RUN-014).
- A preview reaches only the hosts of the app's live release, each with the same key bound the
  same way, and uses the preview's own value of each secret. A host that the preview's publish
  adds, or binds another way, is refused in the preview with E-OUT-001 until a release with it is
  live. A preview of an app with no live release reaches no outside service.

## See also

- [Call an outside service](../tasks/call-an-outside-service.md): the steps, with the example's
  code.
- [The permission list](permission-list.md): the `egress` and `secrets` sections.
- [Settings and secrets](settings-and-secrets.md): secrets the app reads itself.
- [Telemetry](telemetry.md): spans and what they may hold.
