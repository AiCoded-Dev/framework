package dev

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"regexp"
	"time"

	"github.com/go-sql-driver/mysql"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/migrate"
	"aicoded.dev/framework/runnerproto/mysqlproxy"
)

// prepareTimeout bounds Prepare.
const prepareTimeout = 30 * time.Second

// errNoTable is MySQL's error number for a table that does not exist.
const errNoTable = 1146

// MySQLAdmin prepares app databases on the developer's MySQL server.
type MySQLAdmin struct {
	cfg     *mysql.Config
	conn    driver.Connector
	timeout time.Duration
}

// NewMySQLAdmin checks the admin DSN from dev.yaml. The server must be on this machine: a Unix
// socket, or TCP to 127.0.0.1 or ::1. Errors never quote the DSN, which holds a password.
func NewMySQLAdmin(dsn string) (*MySQLAdmin, error) {
	cfg, err := mysql.ParseDSN(dsn)
	var conn driver.Connector
	if err == nil {
		conn, err = mysql.NewConnector(cfg)
	}
	if err != nil {
		return nil, errs.New("E-DEV-007", "the MySQL admin DSN in dev.yaml is not valid",
			"write it as user:password@unix(/var/run/mysqld/mysqld.sock)/ or user:password@tcp(127.0.0.1:3306)/")
	}
	switch cfg.Net {
	case "unix":
	case "tcp", "tcp4", "tcp6":
		host, _, err := net.SplitHostPort(cfg.Addr)
		if ip := net.ParseIP(host); err != nil || ip == nil || !ip.IsLoopback() {
			return nil, errs.New("E-DEV-007", "the MySQL admin DSN in dev.yaml must reach a server on this machine",
				"use a Unix socket, or tcp(127.0.0.1:3306) or tcp([::1]:3306)")
		}
	default:
		return nil, errs.New("E-DEV-007", "the MySQL admin DSN in dev.yaml must use a Unix socket or TCP",
			"use unix(/path/to/mysqld.sock) or tcp(127.0.0.1:3306)")
	}
	return &MySQLAdmin{cfg: cfg, conn: conn, timeout: prepareTimeout}, nil
}

// Users are the two accounts of an app's database.
type Users struct {
	// Data may only read and write rows: it has SELECT, INSERT, UPDATE and DELETE on the app's
	// database and nothing else. The proxy logs in with it.
	Data mysqlproxy.Upstream
	// Owner has every privilege on the app's database and nothing else. Only the migrations run
	// as it; the app never gets it.
	Owner mysqlproxy.Upstream
}

// Prepare creates the database and the two users of app in env when they are missing, gives
// each user a new random password, takes away every privilege it has and grants the data user
// SELECT, INSERT, UPDATE and DELETE and the owner everything on the app's own database only, and
// returns where each logs in. It gives up after 30 seconds.
func (a *MySQLAdmin) Prepare(ctx context.Context, app, env string) (Users, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	db := sql.OpenDB(a.conn)
	defer db.Close()
	name := dbName(app, env)
	users := Users{
		Data:  mysqlproxy.Upstream{Network: a.cfg.Net, Address: a.cfg.Addr, User: userName(app, env), Password: rand.Text(), Database: name},
		Owner: mysqlproxy.Upstream{Network: a.cfg.Net, Address: a.cfg.Addr, User: ownerName(app, env), Password: rand.Text(), Database: name},
	}
	stmts := []string{"CREATE DATABASE IF NOT EXISTS " + quoteName(name) + " CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"}
	for _, u := range []struct {
		up         mysqlproxy.Upstream
		privileges string
	}{
		{users.Data, "SELECT, INSERT, UPDATE, DELETE"},
		{users.Owner, "ALL PRIVILEGES"},
	} {
		who, limit := account(u.up.User), fmt.Sprintf(" WITH MAX_USER_CONNECTIONS %d", mysqlproxy.MaxConns)
		stmts = append(stmts,
			"CREATE USER IF NOT EXISTS "+who+" IDENTIFIED WITH caching_sha2_password BY "+quoteSecret(u.up.Password)+limit,
			"ALTER USER "+who+" IDENTIFIED WITH caching_sha2_password BY "+quoteSecret(u.up.Password)+limit,
			"REVOKE ALL PRIVILEGES, GRANT OPTION FROM "+who,
			"GRANT "+u.privileges+" ON "+quoteName(name)+".* TO "+who,
		)
	}
	if err := a.exec(ctx, db, app, stmts...); err != nil {
		return Users{}, err
	}
	return users, nil
}

// Migrate prepares the database and the users of app in env as Prepare does, and applies steps
// there as the owner with migrate.Apply, destructive steps too. When the database's journal does
// not start steps, or holds more steps than steps, Migrate first drops the database and makes it
// again, since it holds test data; it drops no other database. Every refusal of Apply, such as a list over its limits,
// is returned as it is. It returns the users.
func (a *MySQLAdmin) Migrate(ctx context.Context, app, env string, steps []migrate.Step) (Users, error) {
	users, err := a.Prepare(ctx, app, env)
	if err != nil {
		return Users{}, err
	}
	ran, err := journal(ctx, users.Owner)
	if err != nil {
		return Users{}, err
	}
	if len(ran) > len(steps) || migrate.Extends(ran, steps) != nil {
		if err := a.exec(ctx, nil, app, "DROP DATABASE IF EXISTS "+quoteName(dbName(app, env))); err != nil {
			return Users{}, err
		}
		if users, err = a.Prepare(ctx, app, env); err != nil {
			return Users{}, err
		}
	}
	if err := apply(ctx, users.Owner, steps); err != nil {
		return Users{}, err
	}
	return users, nil
}

// Drop drops the database and the users of app in env. It gives up after 30 seconds.
func (a *MySQLAdmin) Drop(ctx context.Context, app, env string) error {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	return a.exec(ctx, nil, app,
		"DROP DATABASE IF EXISTS "+quoteName(dbName(app, env)),
		"DROP USER IF EXISTS "+account(userName(app, env)),
		"DROP USER IF EXISTS "+account(ownerName(app, env)))
}

// exec runs stmts on db as the admin, or on a new pool when db is nil, and returns E-DEV-008 for
// the first that fails, naming the server's error number only.
func (a *MySQLAdmin) exec(ctx context.Context, db *sql.DB, app string, stmts ...string) error {
	if db == nil {
		db = sql.OpenDB(a.conn)
		defer db.Close()
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			why := reason(err)
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				why = fmt.Sprintf("the server did not answer within %s", a.timeout)
			}
			return errs.New("E-DEV-008", fmt.Sprintf("could not prepare the database of %s: %s", app, why),
				"check that the MySQL server runs and that the admin user in dev.yaml may create databases and users and grant privileges")
		}
	}
	return nil
}

// owner opens the database of up, as its user.
func owner(up mysqlproxy.Upstream) (*sql.DB, error) {
	cfg := mysql.NewConfig()
	cfg.Net, cfg.Addr, cfg.User, cfg.Passwd, cfg.DBName = up.Network, up.Address, up.User, up.Password, up.Database
	cfg.Timeout = prepareTimeout
	cfg.Logger = log.New(io.Discard, "", 0)
	c, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, err
	}
	return sql.OpenDB(c), nil
}

// apply applies steps to the database of up as its user.
func apply(ctx context.Context, up mysqlproxy.Upstream, steps []migrate.Step) error {
	db, err := owner(up)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = migrate.Apply(ctx, db, steps, true)
	return err
}

// journal returns the steps the database of up ran, as its journal holds them, without their
// statements: none when it has no journal.
func journal(ctx context.Context, up mysqlproxy.Upstream) ([]migrate.Step, error) {
	db, err := owner(up)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, "SELECT seq, publish, step, sha256 FROM aicoded_migrations ORDER BY seq")
	if me := (*mysql.MySQLError)(nil); errors.As(err, &me) && me.Number == errNoTable {
		return nil, nil
	}
	if err != nil {
		return nil, unreadJournal(err)
	}
	defer rows.Close()
	var ran []migrate.Step
	for rows.Next() {
		var st migrate.Step
		if err := rows.Scan(&st.Seq, &st.Publish, &st.Index, &st.SHA256); err != nil {
			return nil, unreadJournal(err)
		}
		ran = append(ran, st)
	}
	if err := rows.Err(); err != nil {
		return nil, unreadJournal(err)
	}
	return ran, nil
}

// unreadJournal is E-MIG-005 for a journal that cannot be read.
func unreadJournal(err error) error {
	return errs.New("E-MIG-005", "cannot read the journal aicoded_migrations: "+reason(err), "check that the MySQL server runs, then start the app again")
}

// reason describes a failed statement without its text, which can quote a password.
func reason(err error) string {
	var me *mysql.MySQLError
	var ne *net.OpError
	switch {
	case errors.As(err, &me):
		return fmt.Sprintf("MySQL error %d", me.Number)
	case errors.As(err, &ne):
		return ne.Error()
	}
	return "the server did not answer"
}

// dbName is the database of app in env, ac-<env>-<app>; userName is its data user, and ownerName,
// ao-<env>-<app>, its owner. Each is cut to MySQL's limits with a hash of app and env when
// longer. They hold no "_" or "%", which are wildcards in GRANT.
func dbName(app, env string) string    { return ident("ac-", app, env, 64) }
func userName(app, env string) string  { return ident("ac-", app, env, 32) }
func ownerName(app, env string) string { return ident("ao-", app, env, 32) }

func ident(prefix, app, env string, limit int) string {
	s := prefix + env + "-" + app
	if len(s) <= limit {
		return s
	}
	sum := sha256.Sum256([]byte(env + "/" + app))
	return s[:limit-9] + "-" + hex.EncodeToString(sum[:4])
}

var (
	sqlName   = regexp.MustCompile(`^[a-z0-9-]+$`)
	sqlSecret = regexp.MustCompile(`^[A-Z2-7]+$`)
)

// quoteName, account and quoteSecret build SQL from names and passwords this file made; they
// panic on anything else, so no outside text can reach a statement.
func quoteName(s string) string {
	if !sqlName.MatchString(s) {
		panic("dev: unsafe SQL name")
	}
	return "`" + s + "`"
}

// account is the app's account. Its host is '%': a local MySQL in Docker sees the runner at
// the bridge address, and only the runner knows the password.
func account(user string) string {
	if !sqlName.MatchString(user) {
		panic("dev: unsafe MySQL account")
	}
	return "'" + user + "'@'%'"
}

func quoteSecret(s string) string {
	if !sqlSecret.MatchString(s) {
		panic("dev: unsafe password")
	}
	return "'" + s + "'"
}

// dsnSecrets returns the admin DSN and its password, by the names aicoded dev hides them under.
func dsnSecrets(dsn string) map[string]string {
	secrets := map[string]string{"mysql": dsn}
	if cfg, err := mysql.ParseDSN(dsn); err == nil && cfg.Passwd != "" {
		secrets["mysql password"] = cfg.Passwd
	}
	return secrets
}
