package migrate_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/testmysql"
	"aicoded.dev/framework/migrate"
)

// database returns a connection to a new database as its owner, a user with every right on it
// and none beyond, and an admin connection.
func database(t *testing.T) (owner, admin *sql.DB, name string) {
	admin = testmysql.Admin(t)
	cfg := testmysql.Config(t)
	cfg.User, cfg.Passwd, cfg.DBName = testmysql.User(t, admin, "caching_sha2_password")
	c, err := mysql.NewConnector(cfg)
	require.NoError(t, err)
	owner = sql.OpenDB(c)
	t.Cleanup(func() { _ = owner.Close() })
	return owner, admin, cfg.DBName
}

// steps returns the statements as the steps of publish, the first of them at seq from.
func steps(from int, publish string, statements ...string) []migrate.Step {
	out := make([]migrate.Step, len(statements))
	for i, s := range statements {
		sum := sha256.Sum256([]byte(s))
		out[i] = migrate.Step{Seq: from + i, Publish: publish, Index: i + 1, SHA256: hex.EncodeToString(sum[:]), Statement: s}
	}
	return out
}

// journal returns the seq, publish, step and sha256 of each row of the journal.
func journal(t *testing.T, db *sql.DB) []migrate.Step {
	rows, err := db.QueryContext(t.Context(), "SELECT seq, publish, step, sha256, applied_at FROM aicoded_migrations ORDER BY seq")
	require.NoError(t, err)
	defer rows.Close()
	var out []migrate.Step
	for rows.Next() {
		var s migrate.Step
		var at int64
		require.NoError(t, rows.Scan(&s.Seq, &s.Publish, &s.Index, &s.SHA256, &at))
		assert.InDelta(t, time.Now().UnixMilli(), at, float64(time.Minute.Milliseconds()), "applied_at is in ms")
		out = append(out, s)
	}
	require.NoError(t, rows.Err())
	return out
}

// ran drops the statements of list, which the journal does not keep.
func ran(list []migrate.Step) []migrate.Step {
	out := make([]migrate.Step, len(list))
	for i, s := range list {
		s.Statement = ""
		out[i] = s
	}
	return out
}

func TestApplyRunsWhatTheJournalLacks(t *testing.T) {
	db, admin, name := database(t)
	list := append(steps(1, "p1", "CREATE TABLE a (x INT)", "ALTER TABLE a ADD COLUMN y INT"), steps(3, "p2", "CREATE INDEX i ON a (y)")...)

	applied, err := migrate.Apply(t.Context(), db, list[:2])
	require.NoError(t, err)
	assert.Equal(t, list[:2], applied)

	applied, err = migrate.Apply(t.Context(), db, list)
	require.NoError(t, err)
	assert.Equal(t, list[2:], applied)
	assert.Equal(t, ran(list), journal(t, db))

	var free int
	require.NoError(t, admin.QueryRowContext(t.Context(), "SELECT IS_FREE_LOCK(?)", lockName(name)).Scan(&free))
	assert.Equal(t, 1, free, "Apply releases its lock")
	var timeout int
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT @@session.lock_wait_timeout").Scan(&timeout))
	assert.NotEqual(t, 30, timeout, "Apply's connection does not go back to the pool")
}

func TestApplyRefusesALongList(t *testing.T) {
	db, _, _ := database(t)
	list := steps(1, "p1", make([]string, 2001)...)
	_, err := migrate.Apply(t.Context(), db, list)
	assert.Equal(t, "E-MIG-004", errs.Code(err))
	require.ErrorContains(t, err, "the list holds 2001 steps")
}

func TestApplyEmpty(t *testing.T) {
	db, _, _ := database(t)
	applied, err := migrate.Apply(t.Context(), db, nil)
	require.NoError(t, err)
	assert.Empty(t, applied)
	assert.Empty(t, journal(t, db), "the journal exists")
}

// A release that fails part-way keeps the steps that ran in the journal, and the next try
// continues from the step that failed.
func TestApplyContinuesAfterAFailure(t *testing.T) {
	db, admin, name := database(t)
	list := steps(1, "p1", "CREATE TABLE a (x INT)", "CREATE TABLE b (x INT)", "ALTER TABLE c ADD COLUMN y INT", "CREATE TABLE d (x INT)")

	applied, err := migrate.Apply(t.Context(), db, list)
	assert.Equal(t, "E-MIG-005", errs.Code(err))
	require.ErrorContains(t, err, "step p1#3 failed: MySQL error 1146: ")
	assert.Equal(t, list[:2], applied)
	assert.Equal(t, ran(list[:2]), journal(t, db))

	_, err = admin.ExecContext(t.Context(), "CREATE TABLE `"+name+"`.c (x INT)")
	require.NoError(t, err)
	applied, err = migrate.Apply(t.Context(), db, list)
	require.NoError(t, err)
	assert.Equal(t, list[2:], applied)
	assert.Equal(t, ran(list), journal(t, db))
}

// A step whose row the journal keeps refusing is reported as run but not recorded, which an
// operator must record, and not as a step that failed.
func TestApplyReportsAStepNotRecorded(t *testing.T) {
	db, admin, name := database(t)
	for _, s := range []string{
		"CREATE TABLE `" + name + "`.aicoded_migrations (seq INT PRIMARY KEY, publish VARCHAR(40) NOT NULL, step INT NOT NULL, " +
			"sha256 CHAR(64) NOT NULL, applied_at BIGINT NOT NULL)",
		"CREATE TRIGGER `" + name + "`.refuse BEFORE INSERT ON `" + name + "`.aicoded_migrations FOR EACH ROW " +
			"SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refused'",
	} {
		_, err := admin.ExecContext(t.Context(), s)
		require.NoError(t, err)
	}

	applied, err := migrate.Apply(t.Context(), db, steps(1, "p1", "CREATE TABLE a (x INT)", "CREATE TABLE b (x INT)"))
	assert.Equal(t, "E-MIG-005", errs.Code(err))
	require.ErrorContains(t, err, "step p1#1 ran but was not recorded in the journal aicoded_migrations: MySQL error 1644: refused")
	require.ErrorContains(t, err, "an operator must add the step's row")
	assert.Empty(t, applied)
	var tables int
	require.NoError(t, admin.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME IN ('a', 'b')", name).Scan(&tables))
	assert.Equal(t, 1, tables, "the step ran, and the next did not")
}

// Two appliers at once on one database run each step once.
func TestApplyConcurrently(t *testing.T) {
	db, _, _ := database(t)
	var statements []string
	for _, n := range "abcdefghijklmnopqrst" {
		statements = append(statements, "CREATE TABLE "+string(n)+" (x INT)")
	}
	list := steps(1, "p1", statements...)

	var wg sync.WaitGroup
	results := make([][]migrate.Step, 2)
	failures := make([]error, 2)
	for i := range 2 {
		wg.Go(func() { results[i], failures[i] = migrate.Apply(t.Context(), db, list) })
	}
	wg.Wait()
	require.NoError(t, failures[0])
	require.NoError(t, failures[1])
	assert.Len(t, append(results[0], results[1]...), len(list), "one applier ran every step, the other none")
	assert.Equal(t, ran(list), journal(t, db))
}

// A rollback to a release whose list starts the journal applies nothing; a list that differs
// from the journal is refused before anything runs.
func TestApplyRollback(t *testing.T) {
	db, _, _ := database(t)
	list := steps(1, "p1", "CREATE TABLE a (x INT)", "CREATE TABLE b (x INT)", "CREATE TABLE c (x INT)")
	_, err := migrate.Apply(t.Context(), db, list)
	require.NoError(t, err)

	applied, err := migrate.Apply(t.Context(), db, list[:2])
	require.NoError(t, err)
	assert.Empty(t, applied)

	other := append(steps(1, "p1", "CREATE TABLE a (x INT)", "CREATE TABLE b (y INT)", "CREATE TABLE c (x INT)"), steps(4, "p2", "CREATE TABLE d (x INT)")...)
	applied, err = migrate.Apply(t.Context(), db, other)
	assert.Equal(t, "E-MIG-004", errs.Code(err))
	require.ErrorContains(t, err, "step 2 (p1#2)")
	assert.Empty(t, applied)
	assert.Equal(t, ran(list), journal(t, db))
}

func TestApplyRefusesABadList(t *testing.T) {
	db, admin, name := database(t)
	good := steps(1, "p1", "CREATE TABLE a (x INT)", "CREATE TABLE b (x INT)")
	bad := func(change func([]migrate.Step)) []migrate.Step {
		list := append([]migrate.Step(nil), good...)
		change(list)
		return list
	}
	for reason, tc := range map[string]struct {
		list []migrate.Step
		code string
	}{
		"a hash that differs": {bad(func(l []migrate.Step) { l[1].Statement = "CREATE TABLE c (x INT)" }), "E-MIG-004"},
		"an upper-case hash":  {bad(func(l []migrate.Step) { l[1].SHA256 = "A" + l[1].SHA256[1:] }), "E-MIG-004"},
		"a gap in seq":        {bad(func(l []migrate.Step) { l[1].Seq = 3 }), "E-MIG-004"},
		"no publish":          {bad(func(l []migrate.Step) { l[1].Publish = "" }), "E-MIG-004"},
		"a publish too long":  {bad(func(l []migrate.Step) { l[1].Publish = string(make([]byte, 41)) }), "E-MIG-004"},
		"an index from 0":     {bad(func(l []migrate.Step) { l[1].Index = 0 }), "E-MIG-004"},
		"a destructive step":  {append(good, steps(3, "p2", "DROP TABLE a")...), "E-MIG-003"},
		"a step not allowed":  {append(good, steps(3, "p2", "INSERT INTO a VALUES (1)")...), "E-MIG-002"},
	} {
		applied, err := migrate.Apply(t.Context(), db, tc.list)
		assert.Equal(t, tc.code, errs.Code(err), reason)
		assert.Empty(t, applied, reason)
	}
	var tables int
	require.NoError(t, admin.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = ?", name).Scan(&tables))
	assert.Equal(t, 1, tables, "only the journal: no step ran")
}

// A step that waits on a table another session locks is bounded by lock_wait_timeout and by
// its context, and a step cut off by its context is stopped in the server too.
func TestApplyTimesOut(t *testing.T) {
	db, admin, name := database(t)
	_, err := migrate.Apply(t.Context(), db, steps(1, "p1", "CREATE TABLE t (x INT)"))
	require.NoError(t, err)

	holder, err := admin.Conn(t.Context())
	require.NoError(t, err)
	defer holder.Close()
	_, err = holder.ExecContext(t.Context(), "LOCK TABLES `"+name+"`.t WRITE")
	require.NoError(t, err)

	waiting := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		_, err := migrate.Apply(ctx, db, steps(1, "p1", "CREATE TABLE t (x INT)", "ALTER TABLE t ADD COLUMN y INT"))
		waiting <- err
	}()
	var timeout string
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.NoError(c, admin.QueryRowContext(t.Context(), `SELECT v.VARIABLE_VALUE
			FROM information_schema.PROCESSLIST p
			JOIN performance_schema.threads h ON h.PROCESSLIST_ID = p.ID
			JOIN performance_schema.variables_by_thread v ON v.THREAD_ID = h.THREAD_ID
			WHERE p.DB = ? AND p.STATE = 'Waiting for table metadata lock' AND v.VARIABLE_NAME = 'lock_wait_timeout'`, name).Scan(&timeout))
	}, time.Second, 20*time.Millisecond)
	assert.Equal(t, "30", timeout)

	err = <-waiting
	assert.Equal(t, "E-MIG-005", errs.Code(err))
	require.ErrorContains(t, err, "step p1#2 was stopped while it ran: context deadline exceeded")
	assert.Equal(t, ran(steps(1, "p1", "CREATE TABLE t (x INT)")), journal(t, db))

	assert.Eventually(t, func() bool {
		var n int
		return admin.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.PROCESSLIST WHERE DB = ? AND INFO LIKE 'ALTER%'", name).Scan(&n) == nil && n == 0
	}, 5*time.Second, 20*time.Millisecond, "the server stopped the step")
	_, err = holder.ExecContext(t.Context(), "UNLOCK TABLES")
	require.NoError(t, err)
	var columns int
	require.NoError(t, admin.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = 't'", name).Scan(&columns))
	assert.Equal(t, 1, columns, "the step did not run once the table was free")
}

// lockName returns the name of the migration lock of database.
func lockName(database string) string {
	sum := sha256.Sum256([]byte(database))
	return ("aicoded_migrations:" + hex.EncodeToString(sum[:]))[:64]
}

// hold takes the named lock on a session of admin until the test ends.
func hold(t *testing.T, admin *sql.DB, lock string) {
	conn, err := admin.Conn(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	var got int
	require.NoError(t, conn.QueryRowContext(t.Context(), "SELECT GET_LOCK(?, 0)", lock).Scan(&got))
	require.Equal(t, 1, got)
}

// Each database has its own migration lock: an applier waits only on appliers of its database.
func TestApplyLocksPerDatabase(t *testing.T) {
	one, admin, name := database(t)
	other, _, _ := database(t)
	hold(t, admin, "aicoded_migrations")
	hold(t, admin, lockName(name))

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, err := migrate.Apply(ctx, other, steps(1, "p1", "CREATE TABLE a (x INT)"))
	require.NoError(t, err, "neither the lock of another database nor a lock of another name blocks Apply")

	ctx, cancel = context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, err = migrate.Apply(ctx, one, steps(1, "p1", "CREATE TABLE a (x INT)"))
	assert.Equal(t, "E-MIG-005", errs.Code(err), "Apply waits on its database's lock")
}

func TestApplyNeedsADatabase(t *testing.T) {
	cfg := testmysql.Config(t)
	cfg.DBName = ""
	c, err := mysql.NewConnector(cfg)
	require.NoError(t, err)
	db := sql.OpenDB(c)
	defer db.Close()
	_, err = migrate.Apply(t.Context(), db, nil)
	assert.Equal(t, "E-MIG-005", errs.Code(err))
	require.ErrorContains(t, err, "the connection has no database selected")
}

// Two appliers on two databases of one server do not wait on each other.
func TestApplyOnTwoDatabases(t *testing.T) {
	one, admin, name := database(t)
	other, _, _ := database(t)
	_, err := migrate.Apply(t.Context(), one, steps(1, "p1", "CREATE TABLE t (x INT)"))
	require.NoError(t, err)
	holder, err := admin.Conn(t.Context())
	require.NoError(t, err)
	defer holder.Close()
	_, err = holder.ExecContext(t.Context(), "LOCK TABLES `"+name+"`.t WRITE")
	require.NoError(t, err)

	waiting := make(chan error, 1)
	go func() {
		_, err := migrate.Apply(t.Context(), one, steps(1, "p1", "CREATE TABLE t (x INT)", "ALTER TABLE t ADD COLUMN y INT"))
		waiting <- err
	}()
	require.Eventually(t, func() bool {
		var n int
		return admin.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.PROCESSLIST WHERE DB = ? AND STATE = 'Waiting for table metadata lock'", name).Scan(&n) == nil && n == 1
	}, time.Second, 20*time.Millisecond, "the first applier holds its lock and waits on the table")

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, err = migrate.Apply(ctx, other, steps(1, "p1", "CREATE TABLE a (x INT)"))
	require.NoError(t, err)

	_, err = holder.ExecContext(t.Context(), "UNLOCK TABLES")
	require.NoError(t, err)
	require.NoError(t, <-waiting)
}

func TestExtends(t *testing.T) {
	list := steps(1, "p1", "CREATE TABLE a (x INT)", "CREATE TABLE b (x INT)", "CREATE TABLE c (x INT)")
	assert.NoError(t, migrate.Extends(nil, list))
	assert.NoError(t, migrate.Extends(ran(list[:2]), list), "the list goes on from the journal")
	assert.NoError(t, migrate.Extends(ran(list), list[:1]), "a rollback")
	assert.NoError(t, migrate.Extends(ran(list), nil))

	for field, change := range map[string]func(*migrate.Step){
		"seq":     func(s *migrate.Step) { s.Seq = 9 },
		"publish": func(s *migrate.Step) { s.Publish = "p2" },
		"index":   func(s *migrate.Step) { s.Index = 9 },
		"sha256":  func(s *migrate.Step) { s.SHA256 = steps(1, "p1", "CREATE TABLE z (x INT)")[0].SHA256 },
	} {
		journal := ran(list)
		change(&journal[1])
		assert.Equal(t, "E-MIG-004", errs.Code(migrate.Extends(journal, list)), field)
		assert.Equal(t, "E-MIG-004", errs.Code(migrate.Extends(journal, list[:2])), field)
		assert.Equal(t, "E-MIG-004", errs.Code(migrate.Extends(journal[:2], list)), field)
	}
}

func TestStepJSON(t *testing.T) {
	data, err := json.Marshal(migrate.Step{Seq: 1, Publish: "p1", Index: 2, SHA256: "ab", Statement: "CREATE TABLE a (x INT)"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"seq":1,"publish":"p1","index":2,"sha256":"ab","statement":"CREATE TABLE a (x INT)"}`, string(data))
}
