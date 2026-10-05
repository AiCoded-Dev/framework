# Live values

A live value is rendered on the server like any other, then rendered again and sent to the open
page whenever it changes, over the page's one live connection. The browser gets text or
escaped markup, never a template, and an input can write a value back through a check you write.

## Declaring

```html
<ssr:var name="visitorsOnline" type="int" reactive="true"/>
<ssr:var name="displayName" type="string" reactive="true" client-writable="true"/>
```

- `reactive="true"` lets the server send new values while the page is open. Any type works.
- `client-writable="true"` also lets the page write the value, through `Validate<Name>`.

A page with live values gets a `Subscribe` hook, a `ReactiveState` with a `Set<Name>` method per
live value, and a `reactive_gen.ts` for its script. `Data` sets the first value, as for any
variable.

## Sending values from the server

`Subscribe` runs while the page is open, on its own goroutine, and sends values with
`state.Set<Name>`:

<!-- code: examples/people/pages/home/dataprovider.go DP.Subscribe -->
```go
// Subscribe counts this page among the open home pages while it is open, and shows the count
// whenever a home page opens or closes.
func (p *DP) Subscribe(ctx context.Context, _ *web.Request, state *ReactiveState) error {
	sub := p.d.HomePages.Subscribe()
	defer func() {
		sub.Close()
		p.d.HomePages.Publish(struct{}{})
	}()
	p.d.HomePages.Publish(struct{}{})
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-sub.Updates():
			n := p.d.HomePages.TotalSubs()
			state.SetVisitorsOnline(n)
			state.SetVisitorsTone(tone(n))
		}
	}
}
```

- It must return when `ctx` is done, which happens when the page closes or its connection ends.
  Until it returns, the viewer's live connection stays taken.
- `r.URLParam` works in it, so it can follow the record the page shows.
- An error it returns, or a panic, closes the page's connection, and the browser connects again.
- `Set<Name>` may be called from any goroutine. Only the latest value of each part of the page is
  sent, so a burst of changes costs one update.

## Waking Subscribe from elsewhere

Something else in the app changes the value: another page's form, another viewer. Package
`web/reactive` has two hubs for this. Make them once, in `deps.Deps`, so every page shares them:

- `reactive.NewBroadcast[V]()` sends every value to every subscriber, such as "a user was
  added".
- `reactive.NewTopic[K, V]()` sends a value to the subscribers of one key, such as "this user
  opened a page", keyed by login.

The add form of the people example publishes once the new user is stored:

```go
p.d.UsersAdded.Publish(struct{}{})
```

and the users list counts the users again when it hears it:

<!-- code: examples/people/pages/users/dataprovider.go DP.Subscribe -->
```go
// Subscribe counts the users again whenever one is added.
func (p *DP) Subscribe(ctx context.Context, _ *web.Request, state *ReactiveState) error {
	sub := p.d.UsersAdded.Subscribe()
	defer sub.Close()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-sub.Updates():
			n, err := p.d.CountUsers(ctx)
			if err != nil {
				return err
			}
			state.SetUserCount(n)
		}
	}
}
```

A topic works the same way, with a key: `topic.Publish(login, t)` and
`sub := topic.Subscribe(r.URLParam("login"))`.

- `Publish` never blocks. Each subscription holds one value, and a newer one replaces a value not
  yet read, so a subscriber always sees the latest.
- `Close` is safe to call twice, and a key with no subscription left is dropped.
- With `struct{}` as the value, a signal means "something changed; read it again", which never
  shows a stale value when two changes race.
- `Broadcast.TotalSubs`, `Topic.TotalSubs` and `Topic.Len` count subscriptions.

The hubs live in the app's memory, so they reach the pages this instance of the app serves.

## Values the page writes

A client-writable value needs `Validate<Name>`, which gets every value the page sends and
returns the one to store:

<!-- code: examples/people/pages/home/dataprovider.go DP.ValidateDisplayName -->
```go
// ValidateDisplayName accepts a display name of at most 50 characters.
func (p *DP) ValidateDisplayName(_ context.Context, _ *web.Request, val string) (string, error) {
	if utf8.RuneCountInString(val) > 50 {
		return "", web.Error(http.StatusUnprocessableEntity, "Use at most 50 characters.")
	}
	return val, nil
}
```

- The value it returns is stored and shown, so it may also clean the value up.
- The message of a `web.Error` reaches the page's `ssr.onError`. Any other error is logged, and
  the page gets "The value was not accepted.". A value that does not fit the variable's type gets
  "The value has the wrong type.".
- The stub `aicoded generate` writes refuses every value until you write the check.
- The value is the viewer's own input: never log it.

The page writes the value in one of two ways:

- `ssr:bind` on a native `<input>`, `<select>` or `<textarea>` keeps it in step both ways with no
  script: `<input type="text" ssr:bind="displayName">`. The variable must be declared in the same
  template with `reactive="true" client-writable="true"` (E-GEN-017), and be a `string`, `bool`
  or number (E-GEN-019). `ssr:bind` does not work on `<ssr:input>`, a file input or an input
  whose `type` is a value (E-GEN-018). The input sends its value once the viewer pauses typing.
- `ssr.set("displayName", "Ada")` in the page's `index.ts`, for any type, sent as JSON; see the
  [TypeScript API](typescript-api.md).

## What updates

The generator finds every place in the template that reads a live value:

| Place | What the page gets |
|---|---|
| `{{ count }}` or `{{ a + b }}` in text | the new text, in a `<span>` around it |
| a live value in an attribute | the whole element, rendered again |
| an `ssr:if` chain that reads one | the whole chain, in an `<ssr-block>` element |
| an `ssr:for` that reads one, in its list or its body | the whole loop; a loop of `<tr>` rows is wrapped in a `<tbody>` |

Text is shown as text; elements are markup rendered by the page's escapers. A part inside a
bigger live part is sent with it, once. Keep live parts small: a live loop sends all its rows on
every change.

Some places cannot update, and the generator refuses them:

- a live value in `<title>` or `<textarea>` (E-GEN-034); bind a textarea with `ssr:bind`;
- an `<ssr:form>`, `<ssr:content/>` or `<ssr:assets/>` inside a live part (E-GEN-046);
- a live value inside a form (E-GEN-048).

## The live connection

A page with live values or page calls opens one connection, at its address plus `/__ws`; the
layouts' and the page's live values share it.

- It passes the same checks as the page: the access rules and `Guard`s of every page on the path.
  Then `Data` of the layouts and the page runs again, and the page gets the current value of
  every live part. `Data` there gets a `web.ResponseWriter` whose headers are never sent.
- It is bound to the viewer, and refused when it comes from another site.
- A viewer has at most 8 open connections to an app at once; a ninth page gets 429 "Too many
  pages are open." and does not update.
- It lasts at most 10 minutes. The browser then connects again at once, so the access rules and
  `Guard`s run again.
- When it drops, the browser connects again with a growing delay of up to 30 seconds, and the
  page gets every current value.
- A page may send 20 writes or calls a second, in bursts of up to 40; more closes the connection.
  A page that does not take the server's updates in time is cut off.
- Values live in the connection, not in a session: two tabs have two connections.

## See also

- [TypeScript API](typescript-api.md): `ssr.on`, `ssr.onError` and `ssr.set`.
- [Page calls](page-calls.md): functions a page's script calls over the same connection.
- [people/pages/home](../../examples/people/pages/home): a counter of open pages and a value the
  page writes.
- [people/deps/seen.go](../../examples/people/deps/seen.go): a `reactive.Topic` keyed by login.
- [Add a live value](../tasks/add-live-value.md): values the server sends while the page is
  open, and one the page writes back.
