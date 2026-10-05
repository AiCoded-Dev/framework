# List own records, or all of them for a role

Show each viewer only the records they own, and a viewer with a role every record, with SQL that
stays a constant: one statement for each case, chosen in Go. The example is the list of tickets
in the room-maintenance example, at `/tickets`: an employee sees the tickets they reported, and
`hotel-ops` see every ticket.

## Steps

1. Name the role once, in `deps`, next to the rules that use it:

   <!-- code: examples/room-maintenance/deps/rules.go HotelOps -->
   ```go
   // HotelOps is the role of the people who look after the rooms: they open every ticket and change
   // its status.
   const HotelOps = "hotel-ops"
   ```

2. Read the rows in `deps` with one constant statement for each case, and let the viewer's role
   choose the statement:

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

   <!-- code: examples/room-maintenance/deps/tickets.go columns -->
   ```go
   const columns = "id, room, title, details, status, author, created, updated"
   ```

   <!-- code: examples/room-maintenance/deps/tickets.go scan -->
   ```go
   // scan reads a ticket from a row that holds the columns.
   func scan(row interface{ Scan(...any) error }) (Ticket, error) {
   	var t Ticket
   	err := row.Scan(&t.ID, &t.Room, &t.Title, &t.Details, &t.Status, &t.Author, &t.Created, &t.Updated)
   	return t, err
   }
   ```

   Both statements are constants, since `columns` is one, and the viewer's `Subject` goes in
   with `?`. Never build one statement whose text depends on the role, such as a variable that
   gets `WHERE author = ?` added for everyone without `hotel-ops`: `aicoded check` refuses SQL
   that is not a constant (E-LINT-007). With no viewer, as in code that runs outside a request,
   the function reads nothing.

3. Pass the viewer from the page's `Data`, and tell the template which list it shows:

   <!-- code: examples/room-maintenance/pages/tickets/dataprovider.go DP.Data -->
   ```go
   // Data lists the tickets the viewer may open.
   func (p *DP) Data(ctx context.Context, _ *web.Request, _ web.ResponseWriter, data *RouteData) error {
   	viewer := auth.Viewer(ctx)
   	tickets, err := p.d.TicketsFor(ctx, viewer)
   	if err != nil {
   		return err
   	}
   	data.Tickets = tickets
   	data.Everyone = viewer.HasRole(deps.HotelOps)
   	return nil
   }
   ```

4. Show the rows with `ssr:for`. The heading and the column of authors follow `everyone`:

   <!-- code: examples/room-maintenance/pages/tickets/index.html -->
   ```html
   <ssr:var name="everyone" type="bool"/>
   <ssr:var name="tickets" type="[]Ticket" reactive="true"/>
   <h1>{{ everyone ? 'All tickets' : 'Your tickets' }}</h1>
   <p id="no-tickets" ssr:if="len(tickets) == 0">No tickets yet.</p>
   <table id="tickets" ssr:else>
     <thead>
       <tr><th>Ticket</th><th>Room</th><th>Problem</th><th ssr:if="everyone">Reported by</th><th>Status</th><th>Updated</th></tr>
     </thead>
     <tbody>
       <tr ssr:for="t in tickets" id="ticket-{{ t.ID }}">
         <td>#{{ t.ID }}</td>
         <td>{{ t.Room }}</td>
         <td><a href="/tickets/{{ t.ID }}">{{ t.Title }}</a></td>
         <td ssr:if="everyone">{{ t.Author }}</td>
         <td class="status {{ t.Status }}">{{ t.Status.Label() }}</td>
         <td>{{ t.Updated.Format('2006-01-02 15:04 UTC') }}</td>
       </tr>
     </tbody>
   </table>
   ```

   The list is a live value, so it follows changes while the page is open, as
   [Add a live value](add-live-value.md) shows; a list that need not follow them leaves out
   `reactive="true"`.

The page's access rule admits every employee: the root layout of room-maintenance asks for
`staff`. The role decides only which rows a viewer reads. A page that opens one record checks the
same rule in a `Guard`, as [Check who owns a record](ownership-check.md) shows.

## Check it

- `aicoded check` lints both statements: each is a constant, so E-LINT-007 passes.
- room-maintenance's `deps/deps_test.go` checks each viewer's list against the MySQL server that
  `AICODED_TEST_MYSQL_DSN` names: alice and bob each see only their own tickets, `Alice` and
  `alice ` see none of alice's, a request with no viewer is refused, and dana, with `hotel-ops`,
  sees every ticket.
- With the personas alice (`staff`) and dana (`staff` and `hotel-ops`): alice's `/tickets` says
  "Your tickets" and lists hers, and dana's says "All tickets" and lists every ticket.

## See also

- [List rows from the database](list-from-database.md): a table, a query with placeholders and a
  list.
- [Check who owns a record](ownership-check.md): a `Guard` on the page, and the same check in the
  app that holds the data.
- [SQL database](../guides/sql-database.md)
- [Room maintenance](../../examples/room-maintenance/README.md): the whole app, with the rules
  for opening and changing a ticket.
