package dev

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/cmd/aicoded/internal/migrations"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/testmysql"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/migrate"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/mysqlproxy"
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
	assert.Equal(t, "ao-dev-my-app", ownerName("my-app", "dev"))
	assert.LessOrEqual(t, len(dbName(long, "dev")), 64)
	assert.LessOrEqual(t, len(userName(long, "dev")), 32)
	assert.LessOrEqual(t, len(ownerName(long, "dev")), 32)
	assert.NotEqual(t, userName(long, "dev"), userName(long+"x", "dev"), "long names stay distinct")
	assert.Equal(t, "ac-dev-ab-b-b-b-b-b-b-b-01236d82", userName(long, "dev"), "cut and hashed")
	assert.Equal(t, "ao-dev-ab-b-b-b-b-b-b-b-01236d82", ownerName(long, "dev"), "cut and hashed")
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
			_, _ = admin.ExecContext(context.Background(), "DROP USER IF EXISTS "+account(ownerName(app, env)))
			_, _ = admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS "+quoteName(dbName(app, env)))
		}
	})
}

// prepared returns an admin of the test MySQL and the users of a new app it prepared, which are
// dropped when the test ends.
func prepared(t *testing.T) (*MySQLAdmin, string, Users) {
	a, err := NewMySQLAdmin(testmysql.DSN(t))
	require.NoError(t, err)
	app := testApp()
	drop(t, testmysql.Admin(t), app)
	users, err := a.Prepare(t.Context(), app, "dev")
	require.NoError(t, err)
	return a, app, users
}

func TestPrepare(t *testing.T) {
	admin := testmysql.Admin(t)
	a, appA, upA := prepared(t)
	appB := strings.Replace(appA, "-", "1", 1) // differs only where A has "-", which as "_" would be a GRANT wildcard
	drop(t, admin, appB)
	upB, err := a.Prepare(t.Context(), appB, "dev")
	require.NoError(t, err)

	for _, up := range []Users{upA, upB} {
		_, err = direct(t, up.Owner).ExecContext(t.Context(), "CREATE TABLE notes (v TEXT)")
		require.NoError(t, err, "the owner changes the schema")
		_, err = direct(t, up.Data).ExecContext(t.Context(), "INSERT INTO notes VALUES ('a')")
		require.NoError(t, err, "the data user writes rows")
	}
	for _, up := range []mysqlproxy.Upstream{upA.Data, upA.Owner} {
		_, err = direct(t, up).ExecContext(t.Context(), "SELECT * FROM "+quoteName(upB.Data.Database)+".notes")
		require.ErrorContains(t, err, "denied", "an app never reaches another app's database")
	}

	_, err = admin.ExecContext(t.Context(), "GRANT SELECT ON *.* TO "+account(upA.Data.User))
	require.NoError(t, err)
	again, err := a.Prepare(t.Context(), appA, "dev")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"GRANT USAGE ON *.* TO `" + upA.Data.User + "`@`%`",
		"GRANT SELECT, INSERT, UPDATE, DELETE ON `" + upA.Data.Database + "`.* TO `" + upA.Data.User + "`@`%`",
	}, grants(t, admin, upA.Data.User), "the data user reads and writes rows of its own database only")
	assert.Equal(t, []string{
		"GRANT USAGE ON *.* TO `" + upA.Owner.User + "`@`%`",
		"GRANT ALL PRIVILEGES ON `" + upA.Owner.Database + "`.* TO `" + upA.Owner.User + "`@`%`",
	}, grants(t, admin, upA.Owner.User), "the owner reaches its own database only")
	assert.NotEqual(t, upA.Data.Password, again.Data.Password, "every start sets a new password")
	assert.NotEqual(t, upA.Owner.Password, again.Owner.Password, "every start sets a new password")
	assert.NotEqual(t, again.Data.Password, again.Owner.Password)
	require.Error(t, direct(t, upA.Data).PingContext(t.Context()), "the old password no longer works")
	assert.NoError(t, direct(t, again.Data).PingContext(t.Context()))
}

func TestDataUserChangesNoSchema(t *testing.T) {
	_, _, users := prepared(t)
	_, err := direct(t, users.Owner).ExecContext(t.Context(), "CREATE TABLE notes (v TEXT)")
	require.NoError(t, err)
	for _, stmt := range []string{"CREATE TABLE more (v TEXT)", "ALTER TABLE notes ADD COLUMN w INT", "DROP TABLE notes",
		"CREATE INDEX i ON notes (v(10))", "TRUNCATE TABLE notes"} {
		_, err := direct(t, users.Data).ExecContext(t.Context(), stmt)
		var me *mysql.MySQLError
		require.ErrorAs(t, err, &me, stmt)
		assert.Contains(t, []uint16{1142, 1044}, me.Number, "%s: access denied", stmt)
	}
}

// steps are the steps of a draft of stmts, as aicoded dev makes them.
func steps(stmts ...string) []migrate.Step { return migrations.Append(nil, migrations.Draft, stmts) }

func rows(t *testing.T, up mysqlproxy.Upstream, table string) int {
	var n int
	require.NoError(t, direct(t, up).QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&n))
	return n
}

func TestMigrate(t *testing.T) {
	a, app, _ := prepared(t)
	users, err := a.Migrate(t.Context(), app, "dev", steps("CREATE TABLE notes (v TEXT)"))
	require.NoError(t, err)
	_, err = direct(t, users.Data).ExecContext(t.Context(), "INSERT INTO notes VALUES ('kept')")
	require.NoError(t, err)

	users, err = a.Migrate(t.Context(), app, "dev", steps("CREATE TABLE notes (v TEXT)", "CREATE TABLE tags (v TEXT)"))
	require.NoError(t, err)
	assert.Equal(t, 1, rows(t, users.Data, "notes"), "a draft that grows is applied on top")
	assert.Equal(t, 0, rows(t, users.Data, "tags"))
}

func TestMigrateRecreatesOnlyItsDatabase(t *testing.T) {
	a, app, _ := prepared(t)
	other := testApp()
	drop(t, testmysql.Admin(t), other)
	theirs, err := a.Migrate(t.Context(), other, "dev", steps("CREATE TABLE notes (v TEXT)"))
	require.NoError(t, err)
	_, err = direct(t, theirs.Data).ExecContext(t.Context(), "INSERT INTO notes VALUES ('theirs')")
	require.NoError(t, err)

	for _, next := range [][]migrate.Step{
		steps("CREATE TABLE notes (w TEXT)"),
		steps(),
	} {
		users, err := a.Migrate(t.Context(), app, "dev", steps("CREATE TABLE notes (v TEXT)", "CREATE TABLE tags (v TEXT)"))
		require.NoError(t, err)
		_, err = direct(t, users.Data).ExecContext(t.Context(), "INSERT INTO notes VALUES ('gone')")
		require.NoError(t, err)

		users, err = a.Migrate(t.Context(), app, "dev", next)
		require.NoError(t, err, "a changed draft, or a journal longer than the list")
		var tables int
		require.NoError(t, direct(t, users.Owner).QueryRowContext(t.Context(),
			"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name <> 'aicoded_migrations'").Scan(&tables))
		assert.Equal(t, len(next), tables, "the database was made again with only the new steps")
	}
	assert.Equal(t, 1, rows(t, theirs.Owner, "notes"), "another app's database is untouched")
}

func TestMigrateFails(t *testing.T) {
	a, app, _ := prepared(t)
	_, err := a.Migrate(t.Context(), app, "dev", steps("CREATE TABLE notes (v NOSUCHTYPE)"))
	assert.Equal(t, "E-MIG-005", errs.Code(err))
	require.ErrorContains(t, err, "step draft#1 failed: MySQL error 1064")

	_, err = a.Migrate(t.Context(), app, "dev", steps("DROP VIEW notes"))
	assert.Equal(t, "E-MIG-002", errs.Code(err), "the steps are classified before they run")
}

func TestMigrateAppliesADestructiveStep(t *testing.T) {
	a, app, _ := prepared(t)
	users, err := a.Migrate(t.Context(), app, "dev", steps("CREATE TABLE notes (v TEXT, w TEXT)", "ALTER TABLE notes DROP COLUMN w"))
	require.NoError(t, err)
	var columns int
	require.NoError(t, direct(t, users.Owner).QueryRowContext(t.Context(),
		"SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'notes'").Scan(&columns))
	assert.Equal(t, 1, columns)
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
	dir := copyHello(t, t.TempDir(), m.App)
	writeFile(t, filepath.Join(dir, ".aicoded", "migration.sql"), "CREATE TABLE notes (v TEXT);\n")
	var out syncBuffer
	inst, err := Start(t.Context(), dir, m, helloValues, Services{StateDir: t.TempDir(), MySQL: a}, signer, &out)
	require.NoError(t, err)
	t.Cleanup(inst.Stop)

	sock := filepath.Join(inst.runDir, runnerproto.MySQLSocket)
	var name string
	require.NoError(t, viaSocket(t, sock).QueryRowContext(t.Context(), "SELECT DATABASE()").Scan(&name))
	assert.Equal(t, dbName(m.App, "dev"), name, "the app gets its own database without a credential")
	_, err = viaSocket(t, sock).ExecContext(t.Context(), "INSERT INTO notes VALUES ('a')")
	require.NoError(t, err, "the draft was applied")
	_, err = viaSocket(t, sock).ExecContext(t.Context(), "CREATE TABLE more (v TEXT)")
	require.ErrorContains(t, err, "Error 1142 (42000): CREATE command denied", "the app changes no schema")

	_, err = admin.ExecContext(t.Context(), "DROP USER "+account(userName(m.App, "dev")))
	require.NoError(t, err)
	require.Error(t, viaSocket(t, sock).PingContext(t.Context()))
	assert.Contains(t, out.String(), m.App+" | database: ", "the proxy's errors reach the app's output")
}

func TestStartRefusesTheDraft(t *testing.T) {
	requireGo(t)
	a, err := NewMySQLAdmin(testmysql.DSN(t))
	require.NoError(t, err)
	signer, err := NewSigner()
	require.NoError(t, err)
	m := helloManifest(t)
	m.App = testApp()
	m.Data = []manifest.Data{{Source: "sqldb", Classes: []string{"internal"}}}
	drop(t, testmysql.Admin(t), m.App)
	dir := copyHello(t, t.TempDir(), m.App)
	for text, code := range map[string]string{
		"CREATE TABLE notes (v TEXT);\nDROP VIEW notes;\n":             "E-MIG-002",
		"INSERT INTO notes VALUES ('a');\n":                            "E-MIG-002",
		"CREATE TABLE notes (v TEXT);\nCREATE TABLE notes (w TEXT);\n": "E-MIG-005",
	} {
		writeFile(t, filepath.Join(dir, ".aicoded", "migration.sql"), text)
		_, err = Start(t.Context(), dir, m, helloValues, Services{StateDir: t.TempDir(), MySQL: a}, signer, io.Discard)
		assert.Equal(t, code, errs.Code(err), text)
	}
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
	assert.Contains(t, grants(t, admin, user), "GRANT SELECT, INSERT, UPDATE, DELETE ON `"+user+"`.* TO `"+user+"`@`%`", "the user")
	owner := "ao-preview-" + m.App
	assert.Contains(t, grants(t, admin, owner), "GRANT ALL PRIVILEGES ON `"+user+"`.* TO `"+owner+"`@`%`", "the owner")
}

// platformAnswers is a platform that answers steps, or fails with err.
type platformAnswers struct {
	steps []migrate.Step
	err   error
}

func (p platformAnswers) Migrations(context.Context, string) ([]migrate.Step, error) {
	return p.steps, p.err
}

// sqlWorkspace returns a workspace folder with one app, of a new name, that declares sqldb and
// has text as its draft, and the app's name and folder.
func sqlWorkspace(t *testing.T, text string) (root, app, dir string) {
	root, app = t.TempDir(), testApp()
	drop(t, testmysql.Admin(t), app)
	dir = copyHello(t, filepath.Join(root, app), app)
	writeFile(t, filepath.Join(dir, manifest.FileName), "app: "+app+"\naudience: {internal: [everyone]}\nsettings: [greeting]\nsecrets: [token]\n"+
		"data:\n  - source: sqldb\n    classes: [internal]\n")
	writeFile(t, filepath.Join(dir, ".aicoded", "migration.sql"), text)
	return root, app, dir
}

func TestWorkspaceAppliesPublishedMigrations(t *testing.T) {
	requireGo(t)
	root, app, dir := sqlWorkspace(t, "CREATE TABLE tags (v TEXT);\n")
	published := migrations.Append(nil, "pub_aaaaaaaaaaaaaaaaaaaaaaaaaa", []string{"CREATE TABLE notes (v TEXT)"})
	var out syncBuffer
	w, err := Open(t.Context(), Config{Root: root, State: t.TempDir(), Out: &out, Platform: platformAnswers{steps: published},
		Dev: devconfig.Workspace{MySQL: testmysql.DSN(t), Apps: map[string]devconfig.AppValues{app: helloValues}}})
	require.NoError(t, err)
	t.Cleanup(w.Close)
	require.Equal(t, devapi.Running, appState(t, w, app).State)
	var tables []string
	rows, err := testmysql.Admin(t).QueryContext(t.Context(), "SELECT table_name FROM information_schema.tables WHERE table_schema = ? ORDER BY 1", dbName(app, "dev"))
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		tables = append(tables, name)
	}
	assert.Equal(t, []string{"aicoded_migrations", "notes", "tags"}, tables, "the published steps, then the draft's")
	assert.FileExists(t, filepath.Join(dir, migrations.Cache), "the published steps are cached")
	assert.NotContains(t, out.String(), "could not get the migrations")
}

func TestWorkspaceWarnsWithoutThePlatform(t *testing.T) {
	requireGo(t)
	root, app, _ := sqlWorkspace(t, "CREATE TABLE tags (v TEXT);\n")
	var out syncBuffer
	w, err := Open(t.Context(), Config{Root: root, State: t.TempDir(), Out: &out, Platform: platformAnswers{err: errors.New("down")},
		Dev: devconfig.Workspace{MySQL: testmysql.DSN(t), Apps: map[string]devconfig.AppValues{app: helloValues}}})
	require.NoError(t, err)
	t.Cleanup(w.Close)
	assert.Equal(t, devapi.Running, appState(t, w, app).State)
	assert.Equal(t, 1, strings.Count(out.String(), "could not get the migrations"), "one warning line")
	assert.Contains(t, out.String(), app+" | could not get the migrations "+app+" published from the platform: down; only the draft is applied\n")
}

func TestWorkspaceFailsOnTheDraft(t *testing.T) {
	requireGo(t)
	root, app, _ := sqlWorkspace(t, "CREATE TABLE tags (v TEXT);\nDROP USER tags;\n")
	w, err := Open(t.Context(), Config{Root: root, State: t.TempDir(), Out: io.Discard,
		Dev: devconfig.Workspace{MySQL: testmysql.DSN(t), Apps: map[string]devconfig.AppValues{app: helloValues}}})
	require.NoError(t, err)
	t.Cleanup(w.Close)
	st := appState(t, w, app)
	assert.Equal(t, devapi.Failed, st.State)
	require.Len(t, st.Problems, 1)
	assert.Equal(t, "E-MIG-002", st.Problems[0].Code)
	assert.Equal(t, ".aicoded/migration.sql:2", st.Problems[0].Pos)
}

func TestWorkspaceAppliesADestructiveDraft(t *testing.T) {
	requireGo(t)
	root, app, _ := sqlWorkspace(t, "CREATE TABLE tags (v TEXT, w TEXT);\nALTER TABLE tags DROP COLUMN w;\n")
	w, err := Open(t.Context(), Config{Root: root, State: t.TempDir(), Out: io.Discard,
		Dev: devconfig.Workspace{MySQL: testmysql.DSN(t), Apps: map[string]devconfig.AppValues{app: helloValues}}})
	require.NoError(t, err)
	t.Cleanup(w.Close)
	require.Equal(t, devapi.Running, appState(t, w, app).State)
	var columns int
	require.NoError(t, testmysql.Admin(t).QueryRowContext(t.Context(),
		"SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = ? AND table_name = 'tags'", dbName(app, "dev")).Scan(&columns))
	assert.Equal(t, 1, columns, "the dev database lost the dropped column")
}

// A list that Apply refuses for itself, here one whose hash does not match, stops the app with
// that refusal, and the dev database stays as it was.
func TestMigrateKeepsTheDatabaseOnARefusedList(t *testing.T) {
	a, app, _ := prepared(t)
	users, err := a.Migrate(t.Context(), app, "dev", steps("CREATE TABLE notes (v TEXT)"))
	require.NoError(t, err)
	_, err = direct(t, users.Data).ExecContext(t.Context(), "INSERT INTO notes VALUES ('kept')")
	require.NoError(t, err)

	bad := steps("CREATE TABLE notes (v TEXT)", "CREATE TABLE tags (v TEXT)")
	bad[1].SHA256 = bad[0].SHA256
	_, err = a.Migrate(t.Context(), app, "dev", bad)
	assert.Equal(t, "E-MIG-004", errs.Code(err))
	require.ErrorContains(t, err, "step draft#2: its sha256 does not match its statement")
	users, err = a.Prepare(t.Context(), app, "dev")
	require.NoError(t, err)
	assert.Equal(t, 1, rows(t, users.Data, "notes"), "the database was not made again")
}

// The platform is asked at the app's first start and at a restart a minute later, not at each
// restart.
func TestWorkspaceAsksThePlatformOnce(t *testing.T) {
	requireGo(t)
	root, app, _ := sqlWorkspace(t, "CREATE TABLE tags (v TEXT);\n")
	p := &counted{}
	w, err := Open(t.Context(), Config{Root: root, State: t.TempDir(), Out: io.Discard, Platform: p,
		Dev: devconfig.Workspace{MySQL: testmysql.DSN(t), Apps: map[string]devconfig.AppValues{app: helloValues}}})
	require.NoError(t, err)
	t.Cleanup(w.Close)
	_, err = w.Restart(t.Context(), app)
	require.NoError(t, err)
	assert.Equal(t, int32(1), p.n.Load())
	later := time.Now().Add(time.Minute)
	w.session.now = func() time.Time { return later }
	st, err := w.Restart(t.Context(), app)
	require.NoError(t, err)
	assert.Equal(t, devapi.Running, st.State)
	assert.Equal(t, int32(2), p.n.Load())
}
