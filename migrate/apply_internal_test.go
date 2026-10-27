package migrate

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/testmysql"
)

func TestValidLimits(t *testing.T) {
	list := func(n, size int) []Step {
		out := make([]Step, n)
		for i := range out {
			out[i] = Step{Seq: i + 1, Publish: "p1", Index: i + 1}
		}
		out[0].Statement = strings.Repeat("x", size)
		for i := range out {
			out[i].SHA256 = sha(out[i].Statement)
		}
		return out
	}
	require.NoError(t, valid(list(2000, 1<<20)))
	assert.Equal(t, "E-MIG-004", errs.Code(valid(list(2001, 0))))
	assert.Equal(t, "E-MIG-004", errs.Code(valid(list(1, 1<<20+1))))
}

// A session stopped after a cancelled step is closed, not returned to the pool, and ended in
// the server.
func TestStopDiscardsTheConnection(t *testing.T) {
	pool, other := testmysql.Admin(t), testmysql.Admin(t)
	conn, err := pool.Conn(t.Context())
	require.NoError(t, err)
	s := &session{db: other, conn: conn}
	require.NoError(t, conn.QueryRowContext(t.Context(), "SELECT CONNECTION_ID()").Scan(&s.id))

	s.stop(t.Context())
	assert.Zero(t, pool.Stats().OpenConnections, "the connection did not go back to its pool")
	var n int
	require.NoError(t, other.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.PROCESSLIST WHERE ID = ?", s.id).Scan(&n))
	assert.Zero(t, n, "the session ended in the server")
}

func TestRunAfterTheContextEnded(t *testing.T) {
	db := testmysql.Admin(t)
	conn, err := db.Conn(t.Context())
	require.NoError(t, err)
	defer conn.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err = (&session{db: db, conn: conn}).run(ctx, Step{Publish: "p1", Index: 2, Statement: "CREATE TABLE a (x INT)"})
	assert.Equal(t, "E-MIG-005", errs.Code(err))
	require.ErrorContains(t, err, "step p1#2 was not run: context canceled")
}

// owned returns a session that holds the migration lock of a new database, as the database's
// owner, and an admin connection.
func owned(t *testing.T) (*session, *sql.DB) {
	admin := testmysql.Admin(t)
	cfg := testmysql.Config(t)
	cfg.User, cfg.Passwd, cfg.DBName = testmysql.User(t, admin, "caching_sha2_password")
	c, err := mysql.NewConnector(cfg)
	require.NoError(t, err)
	db := sql.OpenDB(c)
	t.Cleanup(func() { _ = db.Close() })
	conn, err := db.Conn(t.Context())
	require.NoError(t, err)
	s := &session{db: db, conn: conn}
	t.Cleanup(func() { s.close(context.Background()) })
	require.NoError(t, s.open(t.Context()))
	return s, admin
}

// step returns statement as step seq of p1.
func step(seq int, statement string) Step {
	return Step{Seq: seq, Publish: "p1", Index: seq, SHA256: sha(statement), Statement: statement}
}

// rows returns the seq, publish, step and sha256 of each row of s's journal.
func rows(t *testing.T, s *session) []Step {
	got, err := s.journal(t.Context())
	require.NoError(t, err)
	return got
}

// lose ends s's session in the server, as a lost connection does, and waits for its lock to go.
func lose(t *testing.T, s *session, admin *sql.DB) {
	_, err := admin.ExecContext(t.Context(), "KILL ?", s.id)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		var free int
		return admin.QueryRowContext(t.Context(), "SELECT IS_FREE_LOCK(?)", s.lock).Scan(&free) == nil && free == 1
	}, 5*time.Second, 20*time.Millisecond)
}

// A step whose connection is lost after its statement ran is recorded on a new connection, which
// takes the lock over, and the session goes on there.
func TestRecordAfterTheConnectionIsLost(t *testing.T) {
	s, admin := owned(t)
	first, next := step(1, "CREATE TABLE a (x INT)"), step(2, "CREATE TABLE b (x INT)")
	_, err := s.conn.ExecContext(t.Context(), first.Statement)
	require.NoError(t, err)
	lost := s.id
	lose(t, s, admin)

	require.NoError(t, s.record(t.Context(), first))
	assert.NotEqual(t, lost, s.id)
	var holder int64
	require.NoError(t, admin.QueryRowContext(t.Context(), "SELECT IS_USED_LOCK(?)", s.lock).Scan(&holder))
	assert.Equal(t, s.id, holder, "the new connection holds the lock")
	require.NoError(t, s.run(t.Context(), next))
	assert.Equal(t, []Step{{Seq: 1, Publish: "p1", Index: 1, SHA256: first.SHA256}, {Seq: 2, Publish: "p1", Index: 2, SHA256: next.SHA256}}, rows(t, s))
}

// A row the lost connection added before it went is not added twice.
func TestRecordFindsTheRowOfTheLostConnection(t *testing.T) {
	s, admin := owned(t)
	first := step(1, "CREATE TABLE a (x INT)")
	lose(t, s, admin)
	_, err := admin.ExecContext(t.Context(), "INSERT INTO `"+database(t, s)+"`.aicoded_migrations VALUES (1, 'p1', 1, ?, 0)", first.SHA256)
	require.NoError(t, err)

	require.NoError(t, s.record(t.Context(), first))
	assert.Len(t, rows(t, s), 1)
}

// A step whose row cannot be added on a connection that kept the lock is recorded on a new one.
func TestRecordAfterTheRowWasRefused(t *testing.T) {
	s, admin := owned(t)
	refuseRows(t, admin, database(t, s), "IF @refuse = 1 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refused'; END IF")
	_, err := s.conn.ExecContext(t.Context(), "SET @refuse = 1")
	require.NoError(t, err)
	kept := s.id

	require.NoError(t, s.record(t.Context(), step(1, "CREATE TABLE a (x INT)")))
	assert.Equal(t, kept, s.id)
	assert.Len(t, rows(t, s), 1)
}

// When another applier took the lock after the connection was lost, or the journal went on, the
// step is reported as not recorded and nothing is added.
func TestRecordRefusesWhenAnotherApplierMayHaveRun(t *testing.T) {
	for name, meanwhile := range map[string]func(t *testing.T, s *session, admin *sql.DB){
		"the lock is taken": func(t *testing.T, s *session, admin *sql.DB) {
			conn, err := admin.Conn(t.Context())
			require.NoError(t, err)
			t.Cleanup(func() { _ = conn.Close() })
			_, err = conn.ExecContext(t.Context(), "DO GET_LOCK(?, 0)", s.lock)
			require.NoError(t, err)
		},
		"the journal went on": func(t *testing.T, s *session, admin *sql.DB) {
			_, err := admin.ExecContext(t.Context(), "INSERT INTO `"+database(t, s)+"`.aicoded_migrations VALUES (1, 'p2', 1, ?, 0)", sha("x"))
			require.NoError(t, err)
		},
	} {
		t.Run(name, func(t *testing.T) {
			s, admin := owned(t)
			lose(t, s, admin)
			meanwhile(t, s, admin)
			start := time.Now()
			err := s.record(t.Context(), step(1, "CREATE TABLE a (x INT)"))
			assert.Equal(t, "E-MIG-005", errs.Code(err))
			require.ErrorContains(t, err, "step p1#1 ran but was not recorded in the journal aicoded_migrations: ")
			assert.Less(t, time.Since(start), 5*time.Second, "it does not wait for the lock")
			n := 0
			require.NoError(t, admin.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM `"+database(t, s)+"`.aicoded_migrations WHERE publish = 'p1'").Scan(&n))
			assert.Zero(t, n)
		})
	}
}

// database returns the name of s's database.
func database(t *testing.T, s *session) string {
	var name string
	require.NoError(t, s.db.QueryRowContext(t.Context(), "SELECT DATABASE()").Scan(&name))
	return name
}

// refuseRows makes the journal of database run body, SQL of a trigger, before each row it adds.
func refuseRows(t *testing.T, admin *sql.DB, database, body string) {
	_, err := admin.ExecContext(t.Context(), "CREATE TRIGGER `"+database+"`.refuse BEFORE INSERT ON `"+database+"`.aicoded_migrations FOR EACH ROW BEGIN "+body+"; END")
	require.NoError(t, err)
}
