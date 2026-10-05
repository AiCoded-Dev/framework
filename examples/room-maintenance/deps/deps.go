// Package deps holds what the pages share: the database of rooms and tickets, the rules of who
// may open and change a ticket, and the hubs that tell open lists that a ticket changed.
package deps

import (
	"context"
	"database/sql"

	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/sqldb"
	"aicoded.dev/framework/web/reactive"
)

// Deps is made by New and filled by Start before the app serves.
type Deps struct {
	DB *sql.DB
	// byAuthor wakes the lists of an author when one of their tickets changes, and anyTicket the
	// lists of hotel-ops when any ticket does.
	byAuthor  *reactive.Topic[string, struct{}]
	anyTicket *reactive.Broadcast[struct{}]
}

// New returns Deps with its hubs.
func New() *Deps {
	return &Deps{byAuthor: reactive.NewTopic[string, struct{}](), anyTicket: reactive.NewBroadcast[struct{}]()}
}

// TicketChanged tells the open lists that a ticket of author was reported, edited or changed its
// status: author's own lists and the lists of hotel-ops, and no one else's. The signal carries no
// ticket, so each list reads its tickets again with its own viewer's rights.
func (d *Deps) TicketChanged(author string) {
	d.byAuthor.Publish(author, struct{}{})
	d.anyTicket.Publish(struct{}{})
}

// Changes is a subscription to the changes of tickets. Close it when done.
type Changes interface {
	Updates() <-chan struct{}
	Close()
}

// FollowTickets subscribes to the changes of the tickets viewer may open: every ticket for
// hotel-ops, and only their own for anyone else, so that nobody learns when another's ticket
// changes.
func (d *Deps) FollowTickets(viewer auth.Identity) Changes {
	if viewer.HasRole(HotelOps) {
		return d.anyTicket.Subscribe()
	}
	return d.byAuthor.Subscribe(viewer.Subject)
}

// Start opens the database and prepares the rooms and tickets tables.
func (d *Deps) Start(ctx context.Context) error {
	db, err := sqldb.Open(ctx)
	if err != nil {
		return err
	}
	if err := prepare(ctx, db); err != nil {
		return err
	}
	d.DB = db
	return nil
}

// prepare creates the rooms and tickets tables and, when there are no rooms, adds the hotel's
// rooms. An author is the viewer's Subject, which the company login issues with up to 255
// characters, and compares exactly, with no case folding and no trailing-space padding, so
// "Alice" and "alice " are not "alice".
func prepare(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS rooms (
		number SMALLINT UNSIGNED PRIMARY KEY,
		floor TINYINT UNSIGNED NOT NULL
	)`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS tickets (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		room SMALLINT UNSIGNED NOT NULL,
		title VARCHAR(100) NOT NULL,
		details TEXT NOT NULL,
		status ENUM('open', 'in_progress', 'done') NOT NULL,
		author VARCHAR(255) COLLATE utf8mb4_0900_bin NOT NULL,
		created DATETIME NOT NULL,
		updated DATETIME NOT NULL,
		KEY (author),
		FOREIGN KEY (room) REFERENCES rooms (number)
	)`); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var n int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM rooms").Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	for floor := 1; floor <= floors; floor++ {
		for i := 1; i <= roomsPerFloor; i++ {
			if _, err := tx.ExecContext(ctx, "INSERT INTO rooms (number, floor) VALUES (?, ?)", floor*100+i, floor); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
