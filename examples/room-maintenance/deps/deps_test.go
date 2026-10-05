package deps

import (
	"context"
	"crypto/rand"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/auth"
)

func TestRooms(t *testing.T) {
	d := &Deps{DB: testDB(t)}
	ctx := t.Context()
	require.NoError(t, prepare(ctx, d.DB))
	require.NoError(t, prepare(ctx, d.DB), "a second start adds nothing")
	rooms, err := d.Rooms(ctx)
	require.NoError(t, err)
	require.Len(t, rooms, floors*roomsPerFloor)
	assert.Equal(t, Room{Number: 101, Floor: 1}, rooms[0])
	assert.Equal(t, Room{Number: 408, Floor: 4}, rooms[len(rooms)-1])
}

func TestTickets(t *testing.T) {
	d := &Deps{DB: testDB(t)}
	ctx := t.Context()
	require.NoError(t, prepare(ctx, d.DB))
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

	long := strings.Repeat("s", 255)
	id, err := d.AddTicket(ctx, long, 101, "Long subject", "The company login issues subjects of up to 255 characters.")
	require.NoError(t, err)
	assert.Equal(t, []int64{id}, ids(t, d, auth.Identity{Subject: long, Roles: []string{"staff"}}), "a subject of 255 characters is kept whole")
	assert.Empty(t, ids(t, d, auth.Identity{Subject: long[:254], Roles: []string{"staff"}}))
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

// testDB returns a new, empty database on the MySQL server that AICODED_TEST_MYSQL_DSN names,
// and drops it when the test ends. Without the variable the test is skipped, or fails when
// AICODED_E2E_MYSQL is required. Nothing here prints the DSN, which holds a password.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("AICODED_TEST_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("AICODED_E2E_MYSQL") == "required" {
			t.Fatal("AICODED_E2E_MYSQL=required, but AICODED_TEST_MYSQL_DSN is not set")
		}
		t.Skip("AICODED_TEST_MYSQL_DSN is not set")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal("AICODED_TEST_MYSQL_DSN is not a valid DSN")
	}
	admin := open(t, cfg)
	name := "test_" + strings.ToLower(rand.Text()[:12])
	_, err = admin.ExecContext(t.Context(), "CREATE DATABASE `"+name+"`")
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = admin.ExecContext(context.Background(), "DROP DATABASE `"+name+"`") })
	cfg.DBName = name
	cfg.ParseTime, cfg.Loc = true, time.UTC
	return open(t, cfg)
}

func open(t *testing.T, cfg *mysql.Config) *sql.DB {
	t.Helper()
	c, err := mysql.NewConnector(cfg)
	require.NoError(t, err)
	db := sql.OpenDB(c)
	t.Cleanup(func() { _ = db.Close() })
	return db
}
