package pages

import "database/sql"

// Store holds the database.
type Store struct {
	DB    *sql.DB
	Table string
}

//line store.html:1

// Count counts the rows of the store's table.
func (s Store) Count() *sql.Row { return s.DB.QueryRow("SELECT COUNT(*) FROM " + s.Table) }
