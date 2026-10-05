# Room maintenance

The reference app for records with an owner: hotel staff report problems in rooms, each of them
sees and edits only their own tickets, and hotel-ops see every ticket and change its status,
which shows at once in every open list that holds the ticket.

It runs with the other examples, as [the examples' README](../README.md) explains: open
`http://room-maintenance.localhost:8080/` as alice, bob or dana. carol has no `staff` role, so
every page answers her 403.

## Pages

| Page | Access rule | What it does |
|---|---|---|
| `/tickets` | `staff` | lists the viewer's tickets, or every ticket to hotel-ops, with live statuses |
| `/tickets/report` | `staff` | reports a problem in a room |
| `/tickets/<id>` | `staff`, then a `Guard` | shows a ticket to its author and to hotel-ops |
| `/tickets/<id>/edit` | `staff`, the ticket's `Guard`, then its own | changes the title and details, for the author while the ticket is open |
| `/tickets/<id>/status` | `staff` and `hotel-ops`, then the ticket's `Guard` | changes the status |

The root layout asks for `staff`, so that rule covers every page, and `/` opens `/tickets`.
`/tickets/<id>` is a gate: its `Guard` runs before anything the pages below it do, their
forms included.

## Who may open and change a ticket

The rules are plain functions in [deps/rules.go](deps/rules.go), which a test checks without a
runner or a database:

<!-- code: examples/room-maintenance/deps/rules.go CanOpen -->
```go
// CanOpen reports whether viewer may open t: they reported it, or they have the role hotel-ops.
func CanOpen(viewer auth.Identity, t Ticket) bool {
	return viewer.Authenticated() && (viewer.Subject == t.Author || viewer.HasRole(HotelOps))
}
```

<!-- code: examples/room-maintenance/deps/rules.go CanEdit -->
```go
// CanEdit reports whether viewer may change the title and details of t: they reported it, and it
// is still open.
func CanEdit(viewer auth.Identity, t Ticket) bool {
	return viewer.Authenticated() && viewer.Subject == t.Author && t.Status == Open
}
```

The ticket's `Guard` applies `CanOpen`. It answers 403 also for a ticket that does not exist,
for any id the router accepts, so that an employee cannot tell which tickets exist; hotel-ops,
who may open them all, get 404:

<!-- code: examples/room-maintenance/pages/tickets/n_id/dataprovider.go DP.Guard -->
```go
// Guard lets in the ticket's author and hotel-ops, here and on the pages below. Anyone else gets
// 403, also for a ticket that does not exist, so that they cannot tell which tickets exist.
func (p *DP) Guard(ctx context.Context, r *web.Request) error {
	viewer := auth.Viewer(ctx)
	t, err := p.d.Ticket(ctx, r.URLParamInt("id"))
	switch {
	case errors.Is(err, deps.ErrNoTicket) && viewer.HasRole(deps.HotelOps):
		return web.NotFound()
	case errors.Is(err, deps.ErrNoTicket):
		return web.Forbidden()
	case err != nil:
		return err
	case !deps.CanOpen(viewer, t):
		return web.Forbidden()
	}
	return nil
}
```

The edit page's own `Guard` applies `CanEdit`, so hotel-ops change a ticket's status but never
its text. The database checks ownership too:

- the list reads only the viewer's own tickets, unless the viewer has `hotel-ops`, and refuses a
  request with no viewer:

  <!-- code: examples/room-maintenance/deps/tickets.go Deps.TicketsFor -->
  ```go
  // TicketsFor returns the tickets viewer may open, newest first: every ticket to hotel-ops, and
  // only their own to anyone else. With no viewer it returns ErrNoViewer.
  func (d *Deps) TicketsFor(ctx context.Context, viewer auth.Identity) ([]Ticket, error) {
  	if !viewer.Authenticated() {
  		return nil, ErrNoViewer
  	}
  	var rows *sql.Rows
  	var err error
  	if viewer.HasRole(HotelOps) {
  		rows, err = d.DB.QueryContext(ctx, "SELECT "+columns+" FROM tickets ORDER BY id DESC")
  	} else {
  		rows, err = d.DB.QueryContext(ctx, "SELECT "+columns+" FROM tickets WHERE author = ? ORDER BY id DESC", viewer.Subject)
  	}
  	if err != nil {
  		return nil, err
  	}
  	defer rows.Close()
  	var tickets []Ticket
  	for rows.Next() {
  		t, err := scan(rows)
  		if err != nil {
  			return nil, err
  		}
  		tickets = append(tickets, t)
  	}
  	return tickets, rows.Err()
  }
  ```

- an edit changes a ticket only while the viewer is its author and it is open:

  <!-- code: examples/room-maintenance/deps/tickets.go Deps.EditTicket -->
  ```go
  // EditTicket changes the title and details of the ticket id, if author reported it and it is
  // still open; otherwise it changes nothing.
  func (d *Deps) EditTicket(ctx context.Context, id int64, author, title, details string) error {
  	_, err := d.DB.ExecContext(ctx, `UPDATE tickets SET title = ?, details = ?, updated = UTC_TIMESTAMP()
  		WHERE id = ? AND author = ? AND status = 'open'`, title, details, id, author)
  	return err
  }
  ```

The author is the viewer's `Subject`, never a value the form sends. The `author` column is
`VARCHAR(255) COLLATE utf8mb4_0900_bin`: wide enough for any subject the company login issues,
with no case folding and no trailing-space padding, so the database, like the rules, tells
`Alice` and `alice ` from `alice`.

## The live list

The list's statuses are a live value. A list follows only the tickets its viewer may open: an
employee's list follows a `reactive.Topic` keyed by their login, and the lists of hotel-ops follow
a `reactive.Broadcast`. Every form that changes a ticket tells its author's topic and the
broadcast, so an employee's list never even learns that someone else's ticket changed:

<!-- code: examples/room-maintenance/deps/deps.go Deps.TicketChanged -->
```go
// TicketChanged tells the open lists that a ticket of author was reported, edited or changed its
// status: author's own lists and the lists of hotel-ops, and no one else's. The signal carries no
// ticket, so each list reads its tickets again with its own viewer's rights.
func (d *Deps) TicketChanged(author string) {
	d.byAuthor.Publish(author, struct{}{})
	d.anyTicket.Publish(struct{}{})
}
```

<!-- code: examples/room-maintenance/deps/deps.go Deps.FollowTickets -->
```go
// FollowTickets subscribes to the changes of the tickets viewer may open: every ticket for
// hotel-ops, and only their own for anyone else, so that nobody learns when another's ticket
// changes.
func (d *Deps) FollowTickets(viewer auth.Identity) Changes {
	if viewer.HasRole(HotelOps) {
		return d.anyTicket.Subscribe()
	}
	return d.byAuthor.Subscribe(viewer.Subject)
}
```

The signal carries no ticket: the list reads its tickets again with its own viewer's rights. It
reads them once more as soon as it follows them, so that a change made between the page's `Data`
and its `Subscribe` is not lost:

<!-- code: examples/room-maintenance/pages/tickets/dataprovider.go DP.Subscribe -->
```go
// Subscribe follows the tickets the viewer may open and lists them again whenever one of them is
// reported, edited or changes its status. It lists them once as soon as it follows them, so that
// a change made since Data is not missed.
func (p *DP) Subscribe(ctx context.Context, _ *web.Request, state *ReactiveState) error {
	viewer := auth.Viewer(ctx)
	changes := p.d.FollowTickets(viewer)
	defer changes.Close()
	for {
		tickets, err := p.d.TicketsFor(ctx, viewer)
		if err != nil {
			return err
		}
		state.SetTickets(tickets)
		select {
		case <-ctx.Done():
			return nil
		case <-changes.Updates():
		}
	}
}
```

## Data

- `rooms` holds the hotel's 32 rooms, 101 to 408 on four floors. `deps.Start` adds them in one
  transaction when the table is empty, so a second start adds nothing.
- `tickets` holds the room, the title, the details, the status (`open`, `in_progress` or
  `done`), the author's login, and when the ticket was reported and last changed, in UTC. A
  foreign key keeps every ticket to a room the hotel has.
- Every statement is a constant with `?` placeholders. The app logs nothing: its errors go back
  to the framework, which records them without their text outside `aicoded dev`.

## Tests

- `deps/rules_test.go` checks `CanOpen`, `CanEdit`, which lists a change wakes and the limits of
  a ticket's text, and `pages/tickets/report/dataprovider_test.go` how the report form offers the
  rooms.
- `deps/deps_test.go` checks the tables against the MySQL server that `AICODED_TEST_MYSQL_DSN`
  names, skipping without it: the rooms, what each viewer's list holds, exact logins, a subject
  of 255 characters, edits by the author and by anyone else, and status changes. A list asked
  for with no viewer and a status other than open, in progress and done are refused before the
  database is reached.
- The end-to-end tests in `cmd/aicoded/internal/e2e` run it with the other examples under
  `aicoded dev`. Over HTTP, an employee can neither open nor change another's ticket by any page
  or form, and gets the same answer for it as for a ticket that does not exist. An employee
  cannot change the status even of their own ticket, hotel-ops cannot edit a ticket's text, and
  carol opens nothing. In Chrome, a status that hotel-ops change shows at once in two open lists,
  and a ticket one employee reports never shows in another's live list. On the live connection,
  an employee's list is not woken when another's ticket changes, though a list of hotel-ops shows
  the change. A marker typed into every field, on posts the app takes and on posts it refuses,
  reaches no log, span or printed line, and no page, a refused form included, breaks its
  Content-Security-Policy.
