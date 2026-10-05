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
	"net"
	"regexp"
	"time"

	"github.com/go-sql-driver/mysql"

	"aicoded.dev/framework/cmd/aicoded/internal/mysqlproxy"
	"aicoded.dev/framework/internal/errs"
)

// prepareTimeout bounds Prepare.
const prepareTimeout = 30 * time.Second

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

// Prepare creates the database and the user of app in env when they are missing, gives the
// user a new random password, takes away every privilege it has and grants it everything on its
// own database only, and returns where the proxy logs in. It gives up after 30 seconds.
func (a *MySQLAdmin) Prepare(ctx context.Context, app, env string) (mysqlproxy.Upstream, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	db := sql.OpenDB(a.conn)
	defer db.Close()
	name, user, password := dbName(app, env), userName(app, env), rand.Text()
	who, limit := account(user), fmt.Sprintf(" WITH MAX_USER_CONNECTIONS %d", mysqlproxy.MaxConns)
	for _, stmt := range []string{
		"CREATE DATABASE IF NOT EXISTS " + quoteName(name) + " CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci",
		"CREATE USER IF NOT EXISTS " + who + " IDENTIFIED WITH caching_sha2_password BY " + quoteSecret(password) + limit,
		"ALTER USER " + who + " IDENTIFIED WITH caching_sha2_password BY " + quoteSecret(password) + limit,
		"REVOKE ALL PRIVILEGES, GRANT OPTION FROM " + who,
		"GRANT ALL PRIVILEGES ON " + quoteName(name) + ".* TO " + who,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			why := reason(err)
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				why = fmt.Sprintf("the server did not answer within %s", a.timeout)
			}
			return mysqlproxy.Upstream{}, errs.New("E-DEV-008", fmt.Sprintf("could not prepare the database of %s: %s", app, why),
				"check that the MySQL server runs and that the admin user in dev.yaml may create databases and users and grant privileges")
		}
	}
	return mysqlproxy.Upstream{Network: a.cfg.Net, Address: a.cfg.Addr, User: user, Password: password, Database: name}, nil
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

// dbName is the database of app in env, ac-<env>-<app>; userName is its user. Both are cut to
// MySQL's limits with a hash of app and env when longer. They hold no "_" or "%", which are
// wildcards in GRANT.
func dbName(app, env string) string   { return ident(app, env, 64) }
func userName(app, env string) string { return ident(app, env, 32) }

func ident(app, env string, limit int) string {
	s := "ac-" + env + "-" + app
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
