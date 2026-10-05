package deps

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsers(t *testing.T) {
	ctx := t.Context()
	d := &Deps{DB: testDB(t)}
	require.NoError(t, prepare(ctx, d.DB))
	require.NoError(t, prepare(ctx, d.DB), "a second start adds nothing")

	users, err := d.Users(ctx)
	require.NoError(t, err)
	assert.Equal(t, seed, users)
	alice, err := d.User(ctx, "alice")
	require.NoError(t, err)
	assert.Equal(t, "Alice Moreau", alice.Name)
	_, err = d.User(ctx, "Alice")
	require.ErrorIs(t, err, ErrNoUser, "a login matches exactly")
	_, err = d.User(ctx, "alice ")
	require.ErrorIs(t, err, ErrNoUser, "a trailing space is part of the login")

	carol := User{Login: "carol", Name: "Carol Diaz", Age: 41, Team: "Finance", Languages: "English", Remote: true, Shift: 2, Bio: "Carol keeps the books."}
	require.NoError(t, d.AddUser(ctx, carol, nil))
	got, err := d.User(ctx, "carol")
	require.NoError(t, err)
	assert.Equal(t, carol, got)
	require.ErrorIs(t, d.AddUser(ctx, carol, nil), ErrLoginTaken)
	n, err := d.CountUsers(ctx)
	require.NoError(t, err)
	assert.Equal(t, len(seed)+1, n)
}

func TestAddUserRemovesItsPhotosWhenItFails(t *testing.T) {
	ctx := t.Context()
	photos := memStore{MapFS: fstest.MapFS{}, broken: "dave/b.png"}
	d := &Deps{DB: testDB(t)}
	require.NoError(t, prepare(ctx, d.DB))
	dave := User{Login: "dave", Name: "Dave Brown", Age: 30, Team: "IT", Languages: "English", OnSite: true, Shift: 1, Bio: "Dave fixes laptops."}
	two := []Photo{{Name: "a.png", Data: []byte("a")}, {Name: "b.png", Data: []byte("b")}}

	err := addUser(ctx, d.DB, photos, dave, two)
	require.ErrorIs(t, err, fs.ErrInvalid)
	var pe *fs.PathError
	require.ErrorAs(t, err, &pe, "the store's error, as it is")
	assert.Equal(t, "dave/b.png", pe.Path)
	assert.Empty(t, photos.MapFS, "a.png and the folder dave are removed")
	_, err = d.User(ctx, "dave")
	require.ErrorIs(t, err, ErrNoUser)

	photos.MapFS["dave"] = &fstest.MapFile{Mode: fs.ModeDir}
	require.Error(t, addUser(ctx, d.DB, photos, dave, two))
	assert.Equal(t, fstest.MapFS{"dave": {Mode: fs.ModeDir}}, photos.MapFS, "a folder the add did not make stays")
}

func TestAddUserRace(t *testing.T) {
	ctx := t.Context()
	d := &Deps{DB: testDB(t)}
	require.NoError(t, prepare(ctx, d.DB))
	erin := User{Login: "erin", Name: "Erin Hall", Age: 30, Team: "IT", Languages: "English", OnSite: true, Shift: 1, Bio: "Erin runs the help desk."}
	other, err := d.DB.BeginTx(ctx, nil) // another add of erin, which passed its check too
	require.NoError(t, err)
	defer func() { _ = other.Rollback() }()
	require.NoError(t, insert(ctx, other, erin))

	added := make(chan error, 1)
	go func() { added <- d.AddUser(ctx, erin, nil) }()
	require.Eventually(t, func() bool {
		var n int
		err := d.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM performance_schema.data_locks WHERE object_schema = DATABASE() AND lock_status = 'WAITING'").Scan(&n)
		return err == nil && n > 0
	}, 10*time.Second, 10*time.Millisecond, "AddUser passed its check and waits to insert")
	require.NoError(t, other.Commit())
	require.ErrorIs(t, <-added, ErrLoginTaken)
}

func TestLastSeen(t *testing.T) {
	s := NewLastSeen()
	_, ok := s.At("alice")
	assert.False(t, ok)

	sub := s.Follow("alice")
	defer sub.Close()
	noon := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	s.Record("alice", noon)
	s.Record("alice", noon.Add(-time.Minute))
	s.Record("bob", noon.Add(time.Minute))
	at, ok := s.At("alice")
	assert.True(t, ok)
	assert.Equal(t, noon, at, "an earlier time never replaces a later one")
	assert.Equal(t, noon, <-sub.Updates())
	select {
	case v := <-sub.Updates():
		t.Fatalf("got %v, which is not alice's latest page", v)
	default:
	}
}

// memStore is a photoStore in memory. A write of the file broken fails with an error that names
// the file, as the runner's errors do.
type memStore struct {
	fstest.MapFS
	broken string
}

func (s memStore) Mkdir(name string) error {
	s.MapFS[name] = &fstest.MapFile{Mode: fs.ModeDir}
	return nil
}

func (s memStore) WriteFile(name string, data []byte) error {
	if name == s.broken {
		return &fs.PathError{Op: "write", Path: name, Err: fmt.Errorf("%w: no room for %s", fs.ErrInvalid, name)}
	}
	s.MapFS[name] = &fstest.MapFile{Data: data}
	return nil
}

func (s memStore) Remove(name string) error {
	delete(s.MapFS, name)
	return nil
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
