package deps

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/sqldb/sqltest"
)

func TestRooms(t *testing.T) {
	d := &Deps{DB: testDB(t)}
	ctx := t.Context()
	require.NoError(t, fill(ctx, d.DB))
	require.NoError(t, fill(ctx, d.DB), "a second start adds nothing")
	rooms, err := d.Rooms(ctx)
	require.NoError(t, err)
	require.Len(t, rooms, floors*roomsPerFloor)
	assert.Equal(t, Room{Number: 101, Floor: 1}, rooms[0])
	assert.Equal(t, Room{Number: 408, Floor: 4}, rooms[len(rooms)-1])
}

func TestTickets(t *testing.T) {
	d := &Deps{DB: testDB(t)}
	ctx := t.Context()
	require.NoError(t, fill(ctx, d.DB))
	tap, err := d.AddTicket(ctx, "alice", 101, "Leaking tap", "The bathroom tap drips.")
	require.NoError(t, err)
	lamp, err := d.AddTicket(ctx, "bob", 204, "Broken lamp", "The desk lamp does not switch on.")
	require.NoError(t, err)
	_, err = d.AddTicket(ctx, "alice", 999, "No such room", "There is no room 999.")
	require.Error(t, err, "a ticket is for a room the hotel has")

	assert.Equal(t, []int64{tap}, ids(t, d, alice), "staff see only their own tickets")
	assert.Equal(t, []int64{lamp}, ids(t, d, bob))
	assert.Empty(t, ids(t, d, auth.Identity{Subject: "Alice", Roles: []string{"staff"}}), "a login matches exactly")
	assert.Empty(t, ids(t, d, auth.Identity{Subject: "alice ", Roles: []string{"staff"}}), "a trailing space is part of the login")
	_, err = d.TicketsFor(ctx, auth.Identity{})
	require.ErrorIs(t, err, ErrNoViewer, "no viewer sees nothing")
	assert.Equal(t, []int64{lamp, tap}, ids(t, d, dana), "hotel-ops see every ticket, newest first")

	got, err := d.Ticket(ctx, tap)
	require.NoError(t, err)
	assert.Equal(t, Ticket{ID: tap, Room: 101, Title: "Leaking tap", Details: "The bathroom tap drips.", Status: Open,
		Author: "alice", Created: got.Created, Updated: got.Created}, got)
	assert.WithinDuration(t, time.Now(), got.Created, time.Minute, "times are in UTC")
	_, err = d.Ticket(ctx, lamp+1)
	require.ErrorIs(t, err, ErrNoTicket)

	require.NoError(t, d.EditTicket(ctx, tap, "alice", "Leaking hot tap", "It drips all night."))
	require.NoError(t, d.EditTicket(ctx, lamp, "alice", "Mine now", "alice did not report it."))
	assert.Equal(t, "Leaking hot tap", title(t, d, tap), "the author edits an open ticket")
	assert.Equal(t, "Broken lamp", title(t, d, lamp), "an edit by anyone else changes nothing")

	require.NoError(t, d.SetStatus(ctx, tap, InProgress))
	got, err = d.Ticket(ctx, tap)
	require.NoError(t, err)
	assert.Equal(t, InProgress, got.Status)
	require.NoError(t, d.EditTicket(ctx, tap, "alice", "Too late", "It is in progress."))
	assert.Equal(t, "Leaking hot tap", title(t, d, tap), "a ticket that is not open keeps its text")
	require.ErrorIs(t, d.SetStatus(ctx, tap, "closed"), ErrNoStatus, "a status is open, in progress or done")

	long := strings.Repeat("s", 512)
	id, err := d.AddTicket(ctx, long, 101, "Long subject", "A subject is at most 512 bytes.")
	require.NoError(t, err)
	assert.Equal(t, []int64{id}, ids(t, d, auth.Identity{Subject: long, Roles: []string{"staff"}}), "a subject of 512 bytes is kept whole")
	assert.Empty(t, ids(t, d, auth.Identity{Subject: long[:511], Roles: []string{"staff"}}))
}

// ids returns the ids of the tickets that viewer may open.
func ids(t *testing.T, d *Deps, viewer auth.Identity) []int64 {
	t.Helper()
	tickets, err := d.TicketsFor(t.Context(), viewer)
	require.NoError(t, err)
	var out []int64
	for _, ticket := range tickets {
		out = append(out, ticket.ID)
	}
	return out
}

// title returns the title of the ticket id.
func title(t *testing.T, d *Deps, id int64) string {
	t.Helper()
	ticket, err := d.Ticket(t.Context(), id)
	require.NoError(t, err)
	return ticket.Title
}

// testDB returns the test database that aicoded check makes with the tables of
// .aicoded/migration.sql, emptied, or skips the test outside aicoded check.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db := sqltest.Open(t)
	_, err := db.ExecContext(t.Context(), "DELETE FROM tickets")
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), "DELETE FROM rooms")
	require.NoError(t, err)
	return db
}
