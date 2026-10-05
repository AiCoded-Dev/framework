package dev

import (
	"context"
	"crypto/rand"
	"database/sql"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/cmd/aicoded/internal/manifest"
	"aicoded.dev/framework/cmd/aicoded/internal/mysqlproxy"
	"aicoded.dev/framework/cmd/aicoded/internal/testmysql"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/socket"
)

func TestNewMySQLAdmin(t *testing.T) {
	for _, dsn := range []string{
		"root:s3cret@unix(/var/run/mysqld/mysqld.sock)/",
		"root:s3cret@tcp(127.0.0.1:3306)/",
		"root:s3cret@tcp([::1]:3306)/",
	} {
		_, err := NewMySQLAdmin(dsn)
		require.NoError(t, err, dsn)
	}
	for _, dsn := range []string{
		"root:s3cret@tcp(db.example:3306)/",
		"root:s3cret@tcp(10.0.0.5:3306)/",
		"root:s3cret@tcp(localhost:3306)/",
		"root:s3cret@pipe(x)/",
		"root@s3cret(x)/",
		"root:s3cret@tcp(127.0.0.1:3306",
	} {
		_, err := NewMySQLAdmin(dsn)
		assert.Equal(t, "E-DEV-007", errs.Code(err), dsn)
		assert.NotContains(t, err.Error(), "s3cret", "the password never reaches a message")
	}
}

func TestDatabaseNames(t *testing.T) {
	long := "a" + strings.Repeat("b-", 30) + "c"
	assert.Equal(t, "ac-dev-notes", dbName("notes", "dev"))
	assert.Equal(t, "ac-dev-my-app", userName("my-app", "dev"))
	assert.LessOrEqual(t, len(dbName(long, "dev")), 64)
	assert.LessOrEqual(t, len(userName(long, "dev")), 32)
	assert.NotEqual(t, userName(long, "dev"), userName(long+"x", "dev"), "long names stay distinct")
	assert.Equal(t, "ac-dev-ab-b-b-b-b-b-b-b-01236d82", userName(long, "dev"), "cut and hashed")
	for _, s := range []string{"ac_dev_notes", "ac%", ""} {
		assert.Panics(t, func() { quoteName(s) }, "GRANT wildcards never reach a statement: %q", s)
	}
}

func TestReasonHidesTheStatement(t *testing.T) {
	err := &mysql.MySQLError{Number: 1064, Message: "near 'BY 'S3CRET' WITH'"}
	assert.Equal(t, "MySQL error 1064", reason(err))
}

func testApp() string { return "t-" + strings.ToLower(rand.Text()[:10]) }

// drop removes what Prepare created for app, in either environment.
func drop(t *testing.T, admin *sql.DB, app string) {
	t.Cleanup(func() {
		for _, env := range []string{"dev", "preview"} {
			_, _ = admin.ExecContext(context.Background(), "DROP USER IF EXISTS "+account(userName(app, env)))
			_, _ = admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS "+quoteName(dbName(app, env)))
		}
	})
}

func TestPrepare(t *testing.T) {
	admin := testmysql.Admin(t)
	a, err := NewMySQLAdmin(testmysql.DSN(t))
	require.NoError(t, err)
	appA := testApp()
	appB := strings.Replace(appA, "-", "1", 1) // differs only where A has "-", which as "_" would be a GRANT wildcard
	drop(t, admin, appA)
	drop(t, admin, appB)
	upA, err := a.Prepare(t.Context(), appA, "dev")
	require.NoError(t, err)
	upB, err := a.Prepare(t.Context(), appB, "dev")
	require.NoError(t, err)

	for _, up := range []mysqlproxy.Upstream{upA, upB} {
		_, err = direct(t, up).ExecContext(t.Context(), "CREATE TABLE notes (v TEXT)")
		require.NoError(t, err)
	}
	_, err = direct(t, upA).ExecContext(t.Context(), "SELECT * FROM "+quoteName(upB.Database)+".notes")
	require.ErrorContains(t, err, "denied", "an app never reaches another app's database")
	_, err = direct(t, upB).ExecContext(t.Context(), "SELECT * FROM "+quoteName(upA.Database)+".notes")
	require.ErrorContains(t, err, "denied", "an app never reaches another app's database")

	_, err = admin.ExecContext(t.Context(), "GRANT SELECT ON *.* TO "+account(upA.User))
	require.NoError(t, err)
	again, err := a.Prepare(t.Context(), appA, "dev")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"GRANT USAGE ON *.* TO `" + upA.User + "`@`%`",
		"GRANT ALL PRIVILEGES ON `" + upA.Database + "`.* TO `" + upA.User + "`@`%`",
	}, grants(t, admin, upA.User), "the account reaches its own database only")
	assert.NotEqual(t, upA.Password, again.Password, "every start sets a new password")
	require.Error(t, direct(t, upA).PingContext(t.Context()), "the old password no longer works")
	assert.NoError(t, direct(t, again).PingContext(t.Context()))
}

func TestPrepareGivesUp(t *testing.T) {
	var lc net.ListenConfig
	l, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close() // connections wait in its backlog and are never answered
	a, err := NewMySQLAdmin("root:pw@tcp(" + l.Addr().String() + ")/")
	require.NoError(t, err)
	a.timeout = 100 * time.Millisecond
	_, err = a.Prepare(t.Context(), "notes", "dev")
	assert.Equal(t, "E-DEV-008", errs.Code(err))
	require.ErrorContains(t, err, "the server did not answer within 100ms")
}

func TestPrepareHidesTheServersText(t *testing.T) {
	admin := testmysql.Admin(t)
	cfg := testmysql.Config(t)
	cfg.User, cfg.Passwd, _ = testmysql.User(t, admin, "caching_sha2_password")
	a, err := NewMySQLAdmin(cfg.FormatDSN())
	require.NoError(t, err)
	app := testApp()
	drop(t, admin, app)
	_, err = a.Prepare(t.Context(), app, "dev")
	assert.Equal(t, "E-DEV-008", errs.Code(err), "an admin that may not create databases")
	require.ErrorContains(t, err, "MySQL error 1044")
	assert.NotContains(t, err.Error(), cfg.User, "no text of the server")
}

func grants(t *testing.T, admin *sql.DB, user string) []string {
	rows, err := admin.QueryContext(t.Context(), "SHOW GRANTS FOR "+account(user))
	require.NoError(t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var g string
		require.NoError(t, rows.Scan(&g))
		out = append(out, g)
	}
	require.NoError(t, rows.Err())
	return out
}

// direct opens up without the proxy, as the proxy itself logs in.
func direct(t *testing.T, up mysqlproxy.Upstream) *sql.DB {
	cfg := mysql.NewConfig()
	cfg.Net, cfg.Addr, cfg.User, cfg.Passwd, cfg.DBName = up.Network, up.Address, up.User, up.Password, up.Database
	return open(t, cfg)
}

// viaSocket opens the app's database through mysql.sock, as sqldb does.
func viaSocket(t *testing.T, sock string) *sql.DB {
	cfg := mysql.NewConfig()
	cfg.Net, cfg.Addr, cfg.User, cfg.AllowNativePasswords = "unix", sock, "app", true
	return open(t, cfg)
}

func open(t *testing.T, cfg *mysql.Config) *sql.DB {
	c, err := mysql.NewConnector(cfg)
	require.NoError(t, err)
	db := sql.OpenDB(c)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// failingAccept is a listener whose Accept fails as it does when the runner is out of files.
type failingAccept struct{ net.Listener }

func (failingAccept) Accept() (net.Conn, error) { return nil, syscall.EMFILE }

func TestServeDBClosesTheSocketWhenItFails(t *testing.T) {
	dir, err := os.MkdirTemp("", "aicoded-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, runnerproto.MySQLSocket)
	l, err := socket.Listen(t.Context(), sock)
	require.NoError(t, err)

	var logged []error
	serveDB(mysqlproxy.New(mysqlproxy.Upstream{}), failingAccept{l}, func(err error) { logged = append(logged, err) })
	assert.Equal(t, []error{syscall.EMFILE}, logged)
	var d net.Dialer
	_, err = d.DialContext(t.Context(), "unix", sock)
	require.Error(t, err, "the app's connections fail at once")
}

func TestStartNeedsMySQL(t *testing.T) {
	signer, err := NewSigner()
	require.NoError(t, err)
	m := manifest.Manifest{App: "notes", Data: []manifest.Data{{Source: "sqldb", Classes: []string{"internal"}}}}
	_, err = Start(t.Context(), t.TempDir(), m, devconfig.AppValues{}, Services{StateDir: t.TempDir()}, signer, io.Discard)
	assert.Equal(t, "E-DEV-006", errs.Code(err))
}

func TestStartServesTheDatabase(t *testing.T) {
	requireGo(t)
	admin := testmysql.Admin(t)
	a, err := NewMySQLAdmin(testmysql.DSN(t))
	require.NoError(t, err)
	signer, err := NewSigner()
	require.NoError(t, err)
	m := helloManifest(t)
	m.App = testApp()
	m.Data = []manifest.Data{{Source: "sqldb", Classes: []string{"internal"}}}
	drop(t, admin, m.App)
	var out syncBuffer
	inst, err := Start(t.Context(), helloDir(t), m, helloValues, Services{StateDir: t.TempDir(), MySQL: a}, signer, &out)
	require.NoError(t, err)
	t.Cleanup(inst.Stop)

	sock := filepath.Join(inst.runDir, runnerproto.MySQLSocket)
	var name string
	require.NoError(t, viaSocket(t, sock).QueryRowContext(t.Context(), "SELECT DATABASE()").Scan(&name))
	assert.Equal(t, dbName(m.App, "dev"), name, "the app gets its own database without a credential")

	_, err = admin.ExecContext(t.Context(), "DROP USER "+account(userName(m.App, "dev")))
	require.NoError(t, err)
	require.Error(t, viaSocket(t, sock).PingContext(t.Context()))
	assert.Contains(t, out.String(), m.App+" | database: ", "the proxy's errors reach the app's output")
}

func TestStartNamesTheDatabaseByEnv(t *testing.T) {
	requireGo(t)
	admin := testmysql.Admin(t)
	a, err := NewMySQLAdmin(testmysql.DSN(t))
	require.NoError(t, err)
	signer, err := NewSigner()
	require.NoError(t, err)
	m := helloManifest(t)
	m.App = testApp()
	m.Data = []manifest.Data{{Source: "sqldb", Classes: []string{"internal"}}}
	drop(t, admin, m.App)
	svc := Services{StateDir: t.TempDir(), MySQL: a, Env: devconfig.EnvPreview}
	inst, err := Start(t.Context(), helloDir(t), m, helloValues, svc, signer, io.Discard)
	require.NoError(t, err)
	t.Cleanup(inst.Stop)

	var name string
	sock := filepath.Join(inst.runDir, runnerproto.MySQLSocket)
	require.NoError(t, viaSocket(t, sock).QueryRowContext(t.Context(), "SELECT DATABASE()").Scan(&name))
	user := "ac-preview-" + m.App
	assert.Equal(t, user, name, "the database")
	assert.Contains(t, grants(t, admin, user), "GRANT ALL PRIVILEGES ON `"+user+"`.* TO `"+user+"`@`%`", "the user")
}
