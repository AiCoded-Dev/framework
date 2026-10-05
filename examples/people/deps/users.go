package deps

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"slices"
	"strconv"

	"aicoded.dev/framework/telemetry"
)

// User is one person in the users table.
type User struct {
	Login string
	Name  string
	Age   int
	Team  string
	// Languages lists the languages the person speaks, joined with ", ".
	Languages string
	OnSite    bool
	Remote    bool
	// Shift is 1 for the day shift and 2 for the night shift.
	Shift int
	Bio   string
}

// Photo is an uploaded photo: its file name and its content.
type Photo struct {
	Name string
	Data []byte
}

// PhotoFile is a stored photo: its file name and its size in bytes.
type PhotoFile struct {
	Name string
	Size int64
}

var (
	// ErrNoUser is the error of a login no user has.
	ErrNoUser = errors.New("no such user")
	// ErrLoginTaken is the error of adding a user whose login another user has.
	ErrLoginTaken = errors.New("the login is taken")
)

const columns = "login, name, age, team, languages, on_site, remote, shift, bio"

const loginExists = "SELECT EXISTS (SELECT 1 FROM users WHERE login = ?)"

// Users returns every user, in the order they were added.
func (d *Deps) Users(ctx context.Context) ([]User, error) {
	rows, err := d.DB.QueryContext(ctx, "SELECT "+columns+" FROM users ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		u, err := scan(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// User returns the user with login, or ErrNoUser.
func (d *Deps) User(ctx context.Context, login string) (User, error) {
	u, err := scan(d.DB.QueryRowContext(ctx, "SELECT "+columns+" FROM users WHERE login = ?", login))
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNoUser
	}
	return u, err
}

// CountUsers returns the number of users.
func (d *Deps) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := d.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&n)
	return n, err
}

// AddUser adds u and stores its photos as <login>/<file name> in one transaction: the user is
// added only when every photo is stored, and an add that fails removes the photos it stored and
// the folder it made. It returns ErrLoginTaken when another user has the login.
func (d *Deps) AddUser(ctx context.Context, u User, photos []Photo) error {
	return addUser(ctx, d.DB, d.Photos, u, photos)
}

// photoStore is what addUser uses of a *filestore.Store, so that a test can pass a store in
// memory.
type photoStore interface {
	Stat(name string) (fs.FileInfo, error)
	Mkdir(name string) error
	WriteFile(name string, data []byte) error
	Remove(name string) error
}

// addUser is AddUser with the database db and the photo store store.
func addUser(ctx context.Context, db *sql.DB, store photoStore, u User, photos []Photo) (err error) {
	ctx, span := telemetry.Start(ctx, "users.add")
	span.SetAttr("photos", strconv.Itoa(len(photos)))
	defer func() {
		if err != nil {
			span.RecordError(err)
		}
		span.End()
	}()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var taken bool
	if err := tx.QueryRowContext(ctx, loginExists, u.Login).Scan(&taken); err != nil {
		return err
	}
	if taken {
		return ErrLoginTaken
	}
	if err := insert(ctx, tx, u); err != nil {
		// An add of the same login that committed after the check above makes the UNIQUE index
		// refuse this one, with an error that names the login: check again, outside this
		// transaction.
		if db.QueryRowContext(ctx, loginExists, u.Login).Scan(&taken) == nil && taken {
			return ErrLoginTaken
		}
		return err
	}
	var stored []string // the folder this add made and the files it wrote, in that order
	defer func() {
		if err != nil {
			if rerr := removePhotos(store, stored); rerr != nil {
				err = errors.Join(err, rerr)
			}
		}
	}()
	if len(photos) > 0 {
		_, serr := store.Stat(u.Login)
		if err := store.Mkdir(u.Login); err != nil {
			return err
		}
		if errors.Is(serr, fs.ErrNotExist) {
			stored = append(stored, u.Login)
		}
	}
	for _, p := range photos {
		name := u.Login + "/" + p.Name
		if err := store.WriteFile(name, p.Data); err != nil {
			return err
		}
		stored = append(stored, name)
	}
	return tx.Commit()
}

// removePhotos removes names from store, last first, and returns the first error.
func removePhotos(store photoStore, names []string) error {
	var first error
	for _, name := range slices.Backward(names) {
		if err := store.Remove(name); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// PhotosOf returns the photos of login, sorted by file name.
func (d *Deps) PhotosOf(login string) ([]PhotoFile, error) {
	entries, err := d.Photos.ReadDir(login)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	files := make([]PhotoFile, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		files = append(files, PhotoFile{Name: info.Name(), Size: info.Size()})
	}
	return files, nil
}

// scan reads a user from a row that holds the columns.
func scan(row interface{ Scan(...any) error }) (User, error) {
	var u User
	err := row.Scan(&u.Login, &u.Name, &u.Age, &u.Team, &u.Languages, &u.OnSite, &u.Remote, &u.Shift, &u.Bio)
	return u, err
}

// prepare creates the users table and, when it is empty, fills it with seed. A login compares
// exactly, with no case folding and no trailing-space padding, so "Alice" and "alice " are not
// "alice".
func prepare(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS users (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		login VARCHAR(32) COLLATE utf8mb4_0900_bin NOT NULL UNIQUE,
		name VARCHAR(100) NOT NULL,
		age TINYINT UNSIGNED NOT NULL,
		team VARCHAR(40) NOT NULL,
		languages VARCHAR(100) NOT NULL,
		on_site BOOL NOT NULL,
		remote BOOL NOT NULL,
		shift TINYINT UNSIGNED NOT NULL,
		bio TEXT NOT NULL
	)`); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var n int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	for _, u := range seed {
		if err := insert(ctx, tx, u); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// insert adds u to the users table.
func insert(ctx context.Context, tx *sql.Tx, u User) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO users ("+columns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		u.Login, u.Name, u.Age, u.Team, u.Languages, u.OnSite, u.Remote, u.Shift, u.Bio)
	return err
}
