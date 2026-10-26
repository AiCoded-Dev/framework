package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"

	"aicoded.dev/framework/internal/errs"
)

// Step is one statement of an app's migrations.
type Step struct {
	// Seq is the step's place in the app's list, from 1.
	Seq int `json:"seq"`
	// Publish is the id of the publish that added the step, or "draft" locally.
	Publish string `json:"publish"`
	// Index is the step's place within its publish, from 1.
	Index int `json:"index"`
	// SHA256 is the lower-case hex SHA-256 of Statement.
	SHA256 string `json:"sha256"`
	// Statement is the SQL statement.
	Statement string `json:"statement"`
}

// name names the step as <publish>#<index>.
func (s Step) name() string {
	return fmt.Sprintf("%s#%d", s.Publish, s.Index)
}

const (
	// stepTimeout bounds each statement.
	stepTimeout = 5 * time.Minute
	// cleanupTimeout bounds what Apply does after its context ends: writing a journal row,
	// stopping a statement and releasing the lock.
	cleanupTimeout = 30 * time.Second
	// maxPublish is the longest publish id the journal holds.
	maxPublish = 40
	// maxSteps is the most steps a list may hold.
	maxSteps = 2000
	// maxList is the most bytes the statements of a list may hold together.
	maxList = 1 << 20

	createJournal = "CREATE TABLE IF NOT EXISTS aicoded_migrations (seq INT PRIMARY KEY, publish VARCHAR(40) NOT NULL, " +
		"step INT NOT NULL, sha256 CHAR(64) NOT NULL, applied_at BIGINT NOT NULL)"

	listFix = "apply the list as the platform gave it: steps numbered from 1, each with the sha256 of its statement"
	failFix = "after a lock wait or a timeout, release again when the table is less busy; when a draft's statement fails, change the draft"
)

// Apply runs on db the steps that its journal, the table aicoded_migrations, lacks, in order,
// and returns them. It holds the database's migration lock meanwhile, a named lock of
// aicoded_migrations: and the hex SHA-256 of the database's name cut to 64 characters, on one
// connection with lock_wait_timeout 30, and runs each step under a 5-minute context, then
// adds its row to the journal.
//
// It refuses, before running anything, a list whose steps are not numbered from 1 or whose
// hash does not match its statement (E-MIG-004), a list the journal does not agree with
// (E-MIG-004, see Extends) and a step to run that Classify refuses. A step that fails stops
// Apply with E-MIG-005; the journal keeps the steps that ran.
func Apply(ctx context.Context, db *sql.DB, steps []Step) (applied []Step, err error) {
	if err := valid(steps); err != nil {
		return nil, err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, failed("cannot connect to the database", err)
	}
	s := &session{db: db, conn: conn}
	defer s.close(ctx)
	if err := s.open(ctx); err != nil {
		return nil, err
	}
	ran, err := s.journal(ctx)
	if err != nil {
		return nil, err
	}
	if err := Extends(ran, steps); err != nil {
		return nil, err
	}
	if len(ran) >= len(steps) {
		return nil, nil
	}
	todo := steps[len(ran):]
	for _, st := range todo {
		if _, err := Classify(st.Statement); err != nil {
			var coded *errs.Error
			errors.As(err, &coded)
			return nil, errs.New(coded.Code, "step "+st.name()+": "+coded.Msg, coded.Fix)
		}
	}
	for _, st := range todo {
		if err := s.run(ctx, st); err != nil {
			return applied, err
		}
		applied = append(applied, st)
	}
	return applied, nil
}

// Extends returns E-MIG-004 unless ran, the steps a database's journal holds, starts steps, or
// steps starts ran (a rollback). Steps match by Seq, Publish, Index and SHA256.
func Extends(ran, steps []Step) error {
	for i := range min(len(ran), len(steps)) {
		r, s := ran[i], steps[i]
		if r.Seq != s.Seq || r.Publish != s.Publish || r.Index != s.Index || r.SHA256 != s.SHA256 {
			return errs.New("E-MIG-004",
				fmt.Sprintf("the database ran step %d (%s) of other migrations than this list holds there (%s)", r.Seq, r.name(), s.name()),
				"release a publish made on top of the one live, so that its migrations start with the ones the database ran")
		}
	}
	return nil
}

// valid refuses a list over 2000 steps or 1 MiB of statements, or whose steps are not numbered
// from 1, or whose hash does not match.
func valid(steps []Step) error {
	size := 0
	for _, s := range steps {
		size += len(s.Statement)
	}
	if len(steps) > maxSteps || size > maxList {
		return errs.New("E-MIG-004", fmt.Sprintf("the list holds %d steps and %d bytes of statements, more than the %d steps and %d bytes it may",
			len(steps), size, maxSteps, maxList), listFix)
	}
	for i, s := range steps {
		switch {
		case s.Seq != i+1:
			return errs.New("E-MIG-004", fmt.Sprintf("step %d of the list has seq %d", i+1, s.Seq), listFix)
		case s.Publish == "" || len(s.Publish) > maxPublish:
			return errs.New("E-MIG-004", fmt.Sprintf("step %d of the list has a publish id that is empty or longer than %d bytes", s.Seq, maxPublish), listFix)
		case s.Index < 1:
			return errs.New("E-MIG-004", fmt.Sprintf("step %d of the list has index %d", s.Seq, s.Index), listFix)
		case s.SHA256 != sha(s.Statement):
			return errs.New("E-MIG-004", fmt.Sprintf("step %s: its sha256 does not match its statement", s.name()), listFix)
		}
	}
	return nil
}

// session is Apply's one connection to the database.
type session struct {
	db     *sql.DB
	conn   *sql.Conn
	id     int64
	lock   string
	locked bool
}

// open takes the lock, sets the session's lock_wait_timeout and creates the journal.
func (s *session) open(ctx context.Context) error {
	var database sql.NullString
	if err := s.conn.QueryRowContext(ctx, "SELECT CONNECTION_ID(), DATABASE()").Scan(&s.id, &database); err != nil {
		return failed("cannot read the connection's id and database", err)
	}
	if !database.Valid {
		return errs.New("E-MIG-005", "the connection has no database selected", "open the connection with the app's database in its DSN")
	}
	s.lock = lockName(database.String)
	var got sql.NullInt64
	if err := s.conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 60)", s.lock).Scan(&got); err != nil {
		return s.failed(ctx, "cannot take the database's migration lock", err)
	}
	if got.Int64 != 1 {
		return errs.New("E-MIG-005", "another applier held the database's migration lock for 60 seconds", "release again once the migrations under way end")
	}
	s.locked = true
	if _, err := s.conn.ExecContext(ctx, "SET SESSION lock_wait_timeout = 30"); err != nil {
		return s.failed(ctx, "cannot set lock_wait_timeout", err)
	}
	if _, err := s.conn.ExecContext(ctx, createJournal); err != nil {
		return s.failed(ctx, "cannot create the journal aicoded_migrations", err)
	}
	return nil
}

// journal returns the steps the journal holds, without their statements.
func (s *session) journal(ctx context.Context) ([]Step, error) {
	rows, err := s.conn.QueryContext(ctx, "SELECT seq, publish, step, sha256 FROM aicoded_migrations ORDER BY seq")
	if err != nil {
		return nil, s.failed(ctx, "cannot read the journal aicoded_migrations", err)
	}
	defer rows.Close()
	var out []Step
	for rows.Next() {
		var st Step
		if err := rows.Scan(&st.Seq, &st.Publish, &st.Index, &st.SHA256); err != nil {
			return nil, failed("cannot read the journal aicoded_migrations", err)
		}
		out = append(out, st)
	}
	if err := rows.Err(); err != nil {
		return nil, s.failed(ctx, "cannot read the journal aicoded_migrations", err)
	}
	return out, nil
}

// run runs a step under its own context, then adds its row to the journal even when ctx has
// ended.
func (s *session) run(ctx context.Context, st Step) error {
	if err := ctx.Err(); err != nil {
		return errs.New("E-MIG-005", fmt.Sprintf("step %s was not run: %v", st.name(), err), failFix)
	}
	sctx, cancel := context.WithTimeout(ctx, stepTimeout)
	defer cancel()
	if _, err := s.conn.ExecContext(sctx, st.Statement); err != nil {
		switch {
		case ctx.Err() != nil:
			s.stop(ctx)
			return errs.New("E-MIG-005", fmt.Sprintf("step %s was stopped while it ran: %v", st.name(), ctx.Err()), failFix)
		case sctx.Err() != nil:
			s.stop(ctx)
			return errs.New("E-MIG-005", fmt.Sprintf("step %s ran longer than %v and was stopped", st.name(), stepTimeout), failFix)
		}
		return failed("step "+st.name()+" failed", err)
	}
	jctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	_, err := s.conn.ExecContext(jctx, "INSERT INTO aicoded_migrations (seq, publish, step, sha256, applied_at) VALUES (?, ?, ?, ?, ?)",
		st.Seq, st.Publish, st.Index, st.SHA256, time.Now().UnixMilli())
	if err != nil {
		return failed("step "+st.name()+" ran, but its row in the journal aicoded_migrations could not be added", err)
	}
	return nil
}

// failed returns E-MIG-005 for what failed, and stops the session when ctx ended.
func (s *session) failed(ctx context.Context, what string, err error) error {
	if ctx.Err() != nil {
		s.stop(ctx)
	}
	return failed(what, err)
}

// stop ends the session in the server, which goes on with a statement after the driver drops
// a connection whose context ended. The lock goes with the session.
func (s *session) stop(ctx context.Context) {
	s.locked = false
	s.discard()
	kctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	_, _ = s.db.ExecContext(kctx, "KILL ?", s.id)
}

// close releases the lock and closes the connection rather than return it, with the session's
// settings, to db's pool.
func (s *session) close(ctx context.Context) {
	if s.locked {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
		defer cancel()
		_, _ = s.conn.ExecContext(rctx, "DO RELEASE_LOCK(?)", s.lock)
	}
	s.discard()
}

// discard closes the connection rather than return it to db's pool.
func (s *session) discard() {
	_ = s.conn.Raw(func(any) error { return driver.ErrBadConn })
	_ = s.conn.Close()
}

// sha returns the lower-case hex SHA-256 of statement.
func sha(statement string) string {
	sum := sha256.Sum256([]byte(statement))
	return hex.EncodeToString(sum[:])
}

// lockName returns the name of the migration lock of database: aicoded_migrations: and the hex
// SHA-256 of its name, cut to the 64 characters MySQL allows. Apps on one server never wait on
// one another's lock.
func lockName(database string) string {
	return ("aicoded_migrations:" + sha(database))[:64]
}

// failed returns E-MIG-005 for what failed, naming MySQL's error number.
func failed(what string, err error) error {
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return errs.New("E-MIG-005", fmt.Sprintf("%s: MySQL error %d: %s", what, me.Number, me.Message), failFix)
	}
	return errs.New("E-MIG-005", fmt.Sprintf("%s: %v", what, err), failFix)
}
