// Package deps holds what the pages share: the database, the photo store, the team's address,
// and the hubs that carry live values from one page to another.
package deps

import (
	"context"
	"database/sql"

	"aicoded.dev/framework/config"
	"aicoded.dev/framework/filestore"
	"aicoded.dev/framework/sqldb"
	"aicoded.dev/framework/web/reactive"
)

// Deps is made by New and filled by Start before the app serves.
type Deps struct {
	DB     *sql.DB
	Photos *filestore.Store
	// TeamAddress is the setting team_address, where the app mails the team.
	TeamAddress string
	// HomePages has one subscription per open home page, and a signal whenever one opens or
	// closes.
	HomePages *reactive.Broadcast[struct{}]
	// UsersAdded signals that a user was added.
	UsersAdded *reactive.Broadcast[struct{}]
	// Seen records when each viewer last opened a page.
	Seen *LastSeen
}

// New returns Deps with its hubs.
func New() *Deps {
	return &Deps{
		HomePages:  reactive.NewBroadcast[struct{}](),
		UsersAdded: reactive.NewBroadcast[struct{}](),
		Seen:       NewLastSeen(),
	}
}

// Start opens the database and the photo store, prepares the users table and reads the team's
// address.
func (d *Deps) Start(ctx context.Context) error {
	db, err := sqldb.Open(ctx)
	if err != nil {
		return err
	}
	if err := prepare(ctx, db); err != nil {
		return err
	}
	photos, err := filestore.Open(ctx, "photos")
	if err != nil {
		return err
	}
	team, err := config.String(ctx, "team_address")
	if err != nil {
		return err
	}
	d.DB, d.Photos, d.TeamAddress = db, photos, team
	return nil
}
