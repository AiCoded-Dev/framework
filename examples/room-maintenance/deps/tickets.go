package deps

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"

	"aicoded.dev/framework/auth"
)

// Status is where a ticket stands.
type Status string

// The statuses of a ticket, in the order a ticket goes through them.
const (
	Open       Status = "open"
	InProgress Status = "in_progress"
	Done       Status = "done"
)

// Statuses lists every status, in order.
var Statuses = []Status{Open, InProgress, Done}

// Label returns the status as people read it.
func (s Status) Label() string {
	switch s {
	case Open:
		return "Open"
	case InProgress:
		return "In progress"
	case Done:
		return "Done"
	}
	return string(s)
}

// Ticket is a problem reported in a room.
type Ticket struct {
	ID      int64
	Room    int
	Title   string
	Details string
	Status  Status
	// Author is the login of the person who reported it.
	Author  string
	Created time.Time
	Updated time.Time
}

var (
	// ErrNoTicket is the error of an id no ticket has.
	ErrNoTicket = errors.New("no such ticket")
	// ErrNoViewer is the error of a request for tickets with no viewer.
	ErrNoViewer = errors.New("no viewer")
	// ErrNoStatus is the error of a status that is not one of Statuses.
	ErrNoStatus = errors.New("no such status")
)

const columns = "id, room, title, details, status, author, created, updated"

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

// Ticket returns the ticket id, or ErrNoTicket. It checks nothing about the viewer: the page's
// Guard does, with CanOpen.
func (d *Deps) Ticket(ctx context.Context, id int64) (Ticket, error) {
	t, err := scan(d.DB.QueryRowContext(ctx, "SELECT "+columns+" FROM tickets WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Ticket{}, ErrNoTicket
	}
	return t, err
}

// AddTicket adds an open ticket that author reported in room, and returns its id.
func (d *Deps) AddTicket(ctx context.Context, author string, room int, title, details string) (int64, error) {
	res, err := d.DB.ExecContext(ctx, `INSERT INTO tickets (room, title, details, status, author, created, updated)
		VALUES (?, ?, ?, 'open', ?, UTC_TIMESTAMP(), UTC_TIMESTAMP())`, room, title, details, author)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// EditTicket changes the title and details of the ticket id, if author reported it and it is
// still open; otherwise it changes nothing.
func (d *Deps) EditTicket(ctx context.Context, id int64, author, title, details string) error {
	_, err := d.DB.ExecContext(ctx, `UPDATE tickets SET title = ?, details = ?, updated = UTC_TIMESTAMP()
		WHERE id = ? AND author = ? AND status = 'open'`, title, details, id, author)
	return err
}

// SetStatus changes the status of the ticket id to s, or returns ErrNoStatus when s is not one of
// Statuses.
func (d *Deps) SetStatus(ctx context.Context, id int64, s Status) error {
	if !slices.Contains(Statuses, s) {
		return ErrNoStatus
	}
	_, err := d.DB.ExecContext(ctx, "UPDATE tickets SET status = ?, updated = UTC_TIMESTAMP() WHERE id = ?", s, id)
	return err
}

// scan reads a ticket from a row that holds the columns.
func scan(row interface{ Scan(...any) error }) (Ticket, error) {
	var t Ticket
	err := row.Scan(&t.ID, &t.Room, &t.Title, &t.Details, &t.Status, &t.Author, &t.Created, &t.Updated)
	return t, err
}
