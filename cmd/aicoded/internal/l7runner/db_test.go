package l7runner

import (
	"context"
	"crypto/rand"
	"database/sql"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/testmysql"
	"aicoded.dev/framework/runnerproto"
)

// account is an L7 database account, made as the operator makes it: all privileges on the
// databases of a fresh prefix, and nothing else.
type account struct {
	dsn      string // with no database
	user     string
	database string // under the prefix
	outside  string // a database outside the prefix
}

func l7Account(t *testing.T) account {
	admin := testmysql.Admin(t)
	prefix := "l7t" + strings.ToLower(rand.Text()[:10])
	a := account{user: prefix, database: prefix + "_db", outside: "o" + prefix}
	password, who := rand.Text(), "'"+a.user+"'@'%'"
	for _, s := range []string{
		"CREATE DATABASE `" + a.database + "`",
		"CREATE DATABASE `" + a.outside + "`",
		"CREATE USER " + who + " IDENTIFIED BY '" + password + "'",
		"GRANT ALL PRIVILEGES ON `" + prefix + `\_%` + "`.* TO " + who,
	} {
		_, err := admin.ExecContext(context.Background(), s)
		require.NoError(t, err, "prepare the account")
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.Background(), "DROP USER IF EXISTS "+who)
		_, _ = admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS `"+a.database+"`")
		_, _ = admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS `"+a.outside+"`")
	})
	cfg := testmysql.Config(t)
	l7 := mysql.NewConfig()
	l7.Net, l7.Addr, l7.User, l7.Passwd = cfg.Net, cfg.Addr, a.user, password
	a.dsn = l7.FormatDSN()
	return a
}

// viaSocket opens the database through mysql.sock as sqldb does, with no idle connections.
func viaSocket(t *testing.T, sock string) *sql.DB {
	cfg := mysql.NewConfig()
	cfg.Net, cfg.Addr, cfg.User = "unix", sock, "app"
	c, err := mysql.NewConnector(cfg)
	require.NoError(t, err)
	db := sql.OpenDB(c)
	db.SetMaxIdleConns(0)
	return db
}

func TestDatabase(t *testing.T) {
	a := l7Account(t)
	r := folders(t, sqlYAML)
	r.start(t, r.args(r.dsn(t, a.dsn, 0o600, a.database)...))
	sock := filepath.Join(r.runnerDir, runnerproto.MySQLSocket)
	assert.Equal(t, fs.FileMode(0o666), mode(t, sock))

	db := viaSocket(t, sock)
	conn, err := db.Conn(t.Context())
	require.NoError(t, err)
	var name string
	require.NoError(t, conn.QueryRowContext(t.Context(), "SELECT DATABASE()").Scan(&name))
	assert.Equal(t, a.database, name, "the proxy logs in to --mysql-database")
	_, err = conn.ExecContext(t.Context(), "USE `"+a.database+"`")
	require.NoError(t, err)
	_, err = conn.ExecContext(t.Context(), "USE mysql")
	require.ErrorContains(t, err, "1044", "access denied")
	_, err = conn.ExecContext(t.Context(), "USE `"+a.outside+"`")
	require.ErrorContains(t, err, "1044", "a database outside the prefix")
	require.NoError(t, conn.Close())

	_, err = testmysql.Admin(t).ExecContext(t.Context(), "DROP USER '"+a.user+"'@'%'")
	require.NoError(t, err)
	for range 2 {
		require.Error(t, db.PingContext(t.Context()), "the account is gone")
	}
	require.NoError(t, db.Close())
	assert.Equal(t, 0, r.stop())
	assert.Equal(t, "aicoded l7-runner: the runner could not open the app's database\n", r.stderr.String(), "once, without the server's text")
}

func TestDatabaseLoginFails(t *testing.T) {
	a := l7Account(t)
	cfg, err := mysql.ParseDSN(a.dsn)
	require.NoError(t, err)
	password := cfg.Passwd
	cfg.Passwd = "wrong" + password
	r := folders(t, sqlYAML)
	var stderr syncBuffer
	assert.Equal(t, exitFail, Main(t.Context(), r.args(r.dsn(t, cfg.FormatDSN(), 0o600, a.database)...), &stderr))
	assert.Contains(t, stderr.String(), "MySQL error 1045")
	for _, secret := range []string{password, a.user, cfg.Addr} {
		assert.NotContains(t, stderr.String(), secret)
	}
	assert.NoFileExists(t, r.control, "nothing listens")
}
