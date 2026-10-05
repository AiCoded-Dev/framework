# Add a live value

Show a value that changes while the page is open, without reloading it, and let the page send
back a value that the server checks. The examples are from the people example: how many users
there are, when a user last opened a page, how many home pages are open, and the display name
that the home page writes.

## A value the server sends

1. Mark the variable live with `reactive="true"` and show it as usual. `pages/users/index.html`
   declares `<ssr:var name="userCount" type="int" reactive="true"/>` and shows
   `<span id="user-count">{{ userCount }}</span>`. Every place that shows it updates in place.
2. Run `aicoded generate`, or save under `aicoded dev`. The page gets a `Subscribe` hook, and a
   `ReactiveState` with a setter for each live value, here `SetUserCount`. The page opens one
   live connection, which passes the same access rules and `Guard`s as the page.
3. `Data` sets the first value, as for any variable. `Subscribe` runs while the page is open,
   sends each new value with its setter, and returns when `ctx` is done:

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

4. Signal the change where it happens. `deps.New` makes the hub `UsersAdded`, a
   `reactive.Broadcast`, and `ProcessAdd` calls `p.d.UsersAdded.Publish(struct{}{})` once it has
   added a user. A hub reaches the pages served by this app's process. A signal with no value,
   followed by a fresh count, keeps every page right when two changes come at once.

## A value for each record

A `reactive.Topic` is keyed, for example by login, so that a page hears only about its own
record. people's `LastSeen` keeps the time each viewer last opened a page, and publishes it under
their login:

<!-- code: examples/people/deps/seen.go -->
```go
package deps

import (
	"sync"
	"time"

	"aicoded.dev/framework/web/reactive"
)

// LastSeen records when each viewer last opened a page, and tells the pages that follow them.
type LastSeen struct {
	mu    sync.Mutex
	at    map[string]time.Time
	topic *reactive.Topic[string, time.Time]
}

// NewLastSeen returns an empty LastSeen.
func NewLastSeen() *LastSeen {
	return &LastSeen{at: map[string]time.Time{}, topic: reactive.NewTopic[string, time.Time]()}
}

// Record notes that login opened a page at t, unless a later time is recorded.
func (s *LastSeen) Record(login string, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.After(s.at[login]) {
		s.at[login] = t
		s.topic.Publish(login, t)
	}
}

// At returns when login last opened a page, and false when they have not since the app started.
func (s *LastSeen) At(login string) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.at[login]
	return t, ok
}

// Follow subscribes to login: the subscription gets the time of every page they open. Close it
// when done.
func (s *LastSeen) Follow(login string) *reactive.TopicSub[time.Time] {
	return s.topic.Subscribe(login)
}
```

The info tab of a user follows that user's login:

<!-- code: examples/people/pages/users/s_login/info/dataprovider.go DP.Subscribe -->
```go
// Subscribe shows the time of every page the user opens while this page is open.
func (p *DP) Subscribe(ctx context.Context, r *web.Request, state *ReactiveState) error {
	sub := p.d.Seen.Follow(r.URLParam("login"))
	defer sub.Close()
	for {
		select {
		case <-ctx.Done():
			return nil
		case at := <-sub.Updates():
			state.SetLastSeen(seen(at, true))
		}
	}
}
```

## Counting the open pages

A subscription lasts as long as its page, so the `TotalSubs` of a `Broadcast` counts the open
pages. The home page publishes when it opens and when it closes, and every open home page then
reads the count again. Its `Data` shows `TotalSubs()+1`, since `Data` runs before `Subscribe`
joins.

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

## A value the page writes

1. Add `client-writable="true"` to the live variable, and bind an input to it with `ssr:bind`:
   `<ssr:var name="displayName" type="string" reactive="true" client-writable="true"/>` and
   `<input id="display-name" type="text" ssr:bind="displayName">`.
2. The page gets a `Validate<Name>` hook. The value it returns is the one the page keeps; an
   error refuses the value. Until you write it, it refuses every value:

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

3. The message of a `web.Error` reaches the page; any other error is logged, and the page gets a
   general message. Show it from `index.ts` next to the template, which imports the page's typed
   `ssr` object from `./reactive_gen`. Its last lines read the data that the template passes with
   `<ssr:json>`.

   <!-- code: examples/people/pages/home/index.ts -->
   ```ts
   import { ssr } from "./reactive_gen";

   const error = document.getElementById("display-name-error");
   ssr.onError((name, message) => {
     if (name === "displayName" && error) error.textContent = message;
   });
   ssr.on("displayName", () => {
     if (error) error.textContent = "";
   });

   const langs: string[] = JSON.parse(document.getElementById("langs-data")?.textContent ?? "[]");
   const note = document.getElementById("script-note");
   if (note) note.textContent = `A script read ${langs.length} languages from the page.`;
   ```

## Check it

- `aicoded check` generates, builds, vets, lints and tests the app.
- Open `http://people.localhost:8080/home` in two tabs: both show 2 open home pages, and 1 again
  when one closes. Type more than 50 characters as the display name to see the error.
- A viewer has at most 8 live connections at a time. The dev UI's Logs page shows an error that
  `Subscribe` returns, and one that a `Validate` hook returns other than a `web.Error`.

## See also

- [Live values](../guides/live-values.md)
- [The TypeScript API](../guides/typescript-api.md)
