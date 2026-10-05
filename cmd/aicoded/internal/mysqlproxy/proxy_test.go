package mysqlproxy

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/testmysql"
)

// serveProxy serves a proxy to up on a Unix socket and returns the socket's path. onError, if
// set, becomes the proxy's OnError before it serves.
func serveProxy(t *testing.T, up Upstream, onError ...func(error)) (string, *Proxy) {
	sock := filepath.Join(shortDir(t), "mysql.sock")
	var lc net.ListenConfig
	l, err := lc.Listen(t.Context(), "unix", sock)
	require.NoError(t, err)
	p := New(up)
	if len(onError) > 0 {
		p.OnError = onError[0]
	}
	go func() { _ = p.Serve(l) }()
	t.Cleanup(func() { _ = l.Close(); p.Close() })
	return sock, p
}

// dialProxy connects to the proxy, closed when the test ends.
func dialProxy(t *testing.T, sock string) net.Conn {
	var d net.Dialer
	c, err := d.DialContext(t.Context(), "unix", sock)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	return c
}

// hello dials the proxy, answers its greeting with caps as user "root" of database "mysql", and
// returns the connection, the greeting and the proxy's answer.
func hello(t *testing.T, sock string, caps uint32) (net.Conn, *greeting, []byte) {
	c := dialProxy(t, sock)
	_, p, err := readPacket(c, maxHandshake)
	require.NoError(t, err)
	if p[0] == 0xff {
		return c, nil, p
	}
	g, err := parseGreeting(p)
	require.NoError(t, err)
	resp := &response{caps: caps | clientConnectWithDB, maxPacket: 1 << 24, charset: 45, user: "root",
		auth: []byte("not a real token"), db: "mysql", plugin: "mysql_native_password"}
	b, err := resp.encode()
	require.NoError(t, err)
	require.NoError(t, writePacket(c, 1, b))
	_, answer, err := readPacket(c, maxHandshake)
	require.NoError(t, err)
	return c, g, answer
}

const appCaps = clientProtocol41 | clientSecureConnection | clientPluginAuth | clientTransactions | clientMultiResults

func TestProxyLogsInAsTheApp(t *testing.T) {
	f := fakeServer(t)
	sock, _ := serveProxy(t, Upstream{Network: "unix", Address: f.sock, User: "ac_dev_notes", Database: "ac_dev_notes"})
	_, g, answer := hello(t, sock, appCaps|clientMultiStatements|clientLocalFiles|clientCompress|clientConnectAttrs)
	assert.Equal(t, byte(0), answer[0], "the app is let in without credentials")
	assert.Zero(t, g.caps&(clientSSL|clientMultiStatements|clientLocalFiles|clientCompress|clientConnectAttrs), "never offered")
	require.Len(t, f.logins(), 1)
	login := f.logins()[0]
	assert.Equal(t, "ac_dev_notes", login.user, "the app's user name is ignored")
	assert.Equal(t, "ac_dev_notes", login.db, "the app's database name is ignored")
	assert.Zero(t, login.caps&(clientMultiStatements|clientLocalFiles|clientCompress|clientConnectAttrs|clientSSL),
		"flags the app asked for but was not offered never reach the server")
	assert.Equal(t, appCaps&session, login.caps&session, "the server's session has exactly the app's flags")
}

func TestProxyFiltersCommands(t *testing.T) {
	f := fakeServer(t)
	sock, _ := serveProxy(t, Upstream{Network: "unix", Address: f.sock, User: "u", Database: "d"})
	c, _, _ := hello(t, sock, appCaps)
	require.NoError(t, writePacket(c, 0, append([]byte{0x03}, "SELECT 1"...)))
	_, ok, err := readPacket(c, maxHandshake)
	require.NoError(t, err)
	assert.Equal(t, byte(0), ok[0], "a query passes")

	for _, cmd := range [][]byte{{0x11}, {0x1b, 0, 0}, {0x12}, {0x1e}, {0x15}, {0x0c}, {0x20}, {0xff}, {}} {
		c, _, _ := hello(t, sock, appCaps)
		require.NoError(t, writePacket(c, 0, cmd))
		_, p, err := readPacket(c, maxHandshake)
		require.NoError(t, err, "%x", cmd)
		assert.Equal(t, byte(0xff), p[0], "%x", cmd)
		assert.Contains(t, string(p), "E-SQL-001", "%x", cmd)
		_, _, err = readPacket(c, maxHandshake)
		require.Error(t, err, "the connection ends after a refused command: %x", cmd)
	}
	for _, p := range f.packets() {
		assert.Equal(t, byte(0x03), p[0], "only the query reached the server")
	}
}

// longQuery returns a query sent as one full packet and a continuation with sequence id seq
// that starts with COM_CHANGE_USER, and the payload the server gets.
func longQuery(seq byte) (packets, payload []byte) {
	payload = append(append([]byte{0x03}, bytes.Repeat([]byte{' '}, maxPayload-1)...), 0x11)
	packets = append([]byte{0xff, 0xff, 0xff, 0}, payload[:maxPayload]...)
	return append(packets, 1, 0, 0, seq, 0x11), payload
}

func TestProxyRelaysLongCommands(t *testing.T) {
	f := fakeServer(t)
	sock, _ := serveProxy(t, Upstream{Network: "unix", Address: f.sock, User: "u", Database: "d"})
	c, _, _ := hello(t, sock, appCaps)
	packets, payload := longQuery(1)
	_, err := c.Write(packets)
	require.NoError(t, err)
	seq, ok, err := readPacket(c, maxHandshake)
	require.NoError(t, err)
	assert.Equal(t, byte(0), ok[0], "a continuation is not judged as a command")
	assert.Equal(t, byte(2), seq)
	require.Len(t, f.packets(), 1)
	assert.True(t, bytes.Equal(payload, f.packets()[0]), "the command reaches the server whole")

	outOfOrder, _ := longQuery(2)
	for name, packets := range map[string][]byte{
		"continuation out of order": outOfOrder,
		"command not at 0":          {9, 0, 0, 3, 0x03, 'S', 'E', 'L', 'E', 'C', 'T', ' ', '1'},
	} {
		c, _, _ := hello(t, sock, appCaps)
		_, err := c.Write(packets)
		require.NoError(t, err, name)
		_, p, err := readPacket(c, maxHandshake)
		require.NoError(t, err, name)
		assert.Contains(t, string(p), "E-SQL-001", name)
		_, _, err = readPacket(c, maxHandshake)
		require.Error(t, err, "%s: the connection ends", name)
	}
	assert.Len(t, f.packets(), 1, "nothing out of order reached the server")
}

func TestProxyRelaysAnEmptyTerminator(t *testing.T) {
	f := fakeServer(t)
	sock, _ := serveProxy(t, Upstream{Network: "unix", Address: f.sock, User: "u", Database: "d"})
	c, _, _ := hello(t, sock, appCaps)
	payload := append([]byte{0x03}, bytes.Repeat([]byte{' '}, maxPayload-1)...)
	_, err := c.Write(append(append([]byte{0xff, 0xff, 0xff, 0}, payload...), 0, 0, 0, 1))
	require.NoError(t, err)
	seq, ok, err := readPacket(c, maxHandshake)
	require.NoError(t, err)
	assert.Equal(t, byte(0), ok[0], "the empty packet ends the command and is not judged as one")
	assert.Equal(t, byte(2), seq)

	require.NoError(t, writePacket(c, 0, append([]byte{0x03}, "SELECT 1"...)))
	_, ok, err = readPacket(c, maxHandshake)
	require.NoError(t, err)
	assert.Equal(t, byte(0), ok[0], "the next command starts at sequence id 0")
	require.Len(t, f.packets(), 2)
	assert.True(t, bytes.Equal(payload, f.packets()[0]), "the command reaches the server whole")
}

func TestProxyJudgesEveryCommandOfASession(t *testing.T) {
	f := fakeServer(t)
	sock, _ := serveProxy(t, Upstream{Network: "unix", Address: f.sock, User: "u", Database: "d"})
	c, _, _ := hello(t, sock, appCaps)
	for _, q := range []string{"SELECT 1", "SELECT 2"} {
		require.NoError(t, writePacket(c, 0, append([]byte{0x03}, q...)))
		_, ok, err := readPacket(c, maxHandshake)
		require.NoError(t, err)
		assert.Equal(t, byte(0), ok[0], "%s passes", q)
	}

	require.NoError(t, writePacket(c, 0, []byte{0x11}))
	seq, p, err := readPacket(c, maxHandshake)
	require.NoError(t, err)
	assert.Equal(t, byte(1), seq)
	assert.Contains(t, string(p), "E-SQL-001", "a forbidden command later in the session is refused")
	_, _, err = readPacket(c, maxHandshake)
	require.Error(t, err, "and the session ends")
	assert.Len(t, f.packets(), 2, "only the queries reached the server")
}

func TestProxyRefusesClients(t *testing.T) {
	f := fakeServer(t)
	sock, _ := serveProxy(t, Upstream{Network: "unix", Address: f.sock, User: "u", Database: "d"})
	valid, err := (&response{caps: appCaps, user: "root"}).encode()
	require.NoError(t, err)
	for name, first := range map[string]struct {
		seq byte
		p   []byte
	}{
		"TLS":          {1, append(binary.LittleEndian.AppendUint32(nil, appCaps|clientSSL), make([]byte, 28)...)},
		"old client":   {1, append(binary.LittleEndian.AppendUint32(nil, clientLongPassword), make([]byte, 28)...)},
		"malformed":    {1, append(binary.LittleEndian.AppendUint32(nil, appCaps), make([]byte, 28)...)},
		"out of order": {2, valid},
		"too large":    {1, make([]byte, maxHandshake+1)},
	} {
		c := dialProxy(t, sock)
		_, _, err := readPacket(c, maxHandshake)
		require.NoError(t, err)
		require.NoError(t, writePacket(c, first.seq, first.p))
		_, p, err := readPacket(c, maxHandshake)
		require.NoError(t, err, name)
		assert.Contains(t, string(p), "E-SQL-003", name)
		_ = c.Close()
	}
	assert.Empty(t, f.logins(), "nothing logs in for a refused client")
}

func TestProxyLimitsConnections(t *testing.T) {
	f := fakeServer(t)
	sock, _ := serveProxy(t, Upstream{Network: "unix", Address: f.sock, User: "u", Database: "d"})
	var first net.Conn
	for range MaxConns {
		c, _, answer := hello(t, sock, appCaps)
		require.Equal(t, byte(0), answer[0])
		if first == nil {
			first = c
		}
	}
	_, _, answer := hello(t, sock, appCaps)
	assert.Contains(t, string(answer), "E-SQL-004")

	_ = first.Close()
	for deadline := time.Now().Add(5 * time.Second); answer[0] != 0 && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
		_, _, answer = hello(t, sock, appCaps)
	}
	assert.Equal(t, byte(0), answer[0], "a closed connection frees its place")
}

func TestProxyUpstreamDown(t *testing.T) {
	valid, err := sha2Server.encode()
	require.NoError(t, err)
	for name, up := range map[string]Upstream{
		"no server":   {Network: "unix", Address: filepath.Join(t.TempDir(), "gone.sock")},
		"login fails": greeter(t, 0, valid),
	} {
		up.User, up.Password, up.Database = "u", "Pa55-never-shown", "d"
		failed := make(chan error, 1)
		sock, _ := serveProxy(t, up, func(err error) { failed <- err })
		_, _, answer := hello(t, sock, appCaps)
		assert.Contains(t, string(answer), "E-SQL-002", name)
		assert.NotContains(t, string(answer), up.Password, name)
		select {
		case err := <-failed:
			assert.NotContains(t, err.Error(), up.Password, name)
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: OnError was not called", name)
		}
	}
}

func TestProxyCloseEndsSessions(t *testing.T) {
	f := fakeServer(t)
	sock, p := serveProxy(t, Upstream{Network: "unix", Address: f.sock, User: "u", Database: "d"})
	session, _, _ := hello(t, sock, appCaps)
	greeted := dialProxy(t, sock)
	_, _, err := readPacket(greeted, maxHandshake)
	require.NoError(t, err)

	start := time.Now()
	p.Close()
	assert.Less(t, time.Since(start), handshakeTimeout, "a session in its handshake ends at once")
	for _, c := range []net.Conn{session, greeted} {
		_, _, err := readPacket(c, maxHandshake)
		require.Error(t, err)
	}
}

// appDB opens the proxy the way sqldb does, with the given driver settings on top.
func appDB(t *testing.T, sock string, change func(*mysql.Config)) *sql.DB {
	cfg := mysql.NewConfig()
	cfg.Net, cfg.Addr, cfg.User, cfg.Passwd, cfg.DBName = "unix", sock, "root", "guess", "mysql"
	cfg.AllowNativePasswords = true
	if change != nil {
		change(cfg)
	}
	c, err := mysql.NewConnector(cfg)
	require.NoError(t, err)
	db := sql.OpenDB(c)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestProxyWithMySQL(t *testing.T) {
	admin := testmysql.Admin(t)
	cfg := testmysql.Config(t)
	user, password, database := testmysql.User(t, admin, "caching_sha2_password")
	_, _, otherDB := testmysql.User(t, admin, "caching_sha2_password")
	_, err := admin.ExecContext(t.Context(), "CREATE TABLE `"+otherDB+"`.secrets (v TEXT)")
	require.NoError(t, err)
	sock, _ := serveProxy(t, Upstream{Network: cfg.Net, Address: cfg.Addr, User: user, Password: password, Database: database})
	db := appDB(t, sock, nil)
	ctx := t.Context()

	var current, schema string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT CURRENT_USER(), DATABASE()").Scan(&current, &schema))
	assert.True(t, strings.HasPrefix(current, user+"@"), "the app is its own user, whatever it sent: %s", current)
	assert.Equal(t, database, schema)
	asSQLDB := appDB(t, sock, func(c *mysql.Config) {
		c.User, c.Passwd, c.DBName, c.Collation = "app", "", "", "utf8mb4_0900_ai_ci"
	})
	require.NoError(t, asSQLDB.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&schema))
	assert.Equal(t, database, schema, "sqldb's client, with no password and no database, gets in")

	_, err = db.ExecContext(ctx, "CREATE TABLE blobs (id INT PRIMARY KEY, b LONGBLOB)")
	require.NoError(t, err)
	big := strings.Repeat("x", 17<<20)
	_, err = db.ExecContext(ctx, "INSERT INTO blobs VALUES (?, ?)", 1, big)
	require.NoError(t, err, "a value over 16 MiB crosses several packets")
	var n int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT LENGTH(b) FROM blobs WHERE id = ?", 1).Scan(&n))
	assert.Equal(t, len(big), n)
	var back []byte
	require.NoError(t, db.QueryRowContext(ctx, "SELECT b FROM blobs WHERE id = ?", 1).Scan(&back))
	assert.Equal(t, sha256.Sum256([]byte(big)), sha256.Sum256(back), "and comes back whole")

	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "INSERT INTO blobs VALUES (2, 'y')")
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM blobs").Scan(&n))
	assert.Equal(t, 1, n)

	_, err = db.ExecContext(ctx, "SELECT * FROM `"+otherDB+"`.secrets")
	require.ErrorContains(t, err, "denied", "another app's database is out of reach")

	multi := appDB(t, sock, func(c *mysql.Config) { c.MultiStatements = true })
	_, err = multi.ExecContext(ctx, "SELECT 1; SELECT 2")
	require.Error(t, err, "multi-statements stay off")

	local := appDB(t, sock, func(c *mysql.Config) { c.AllowAllFiles = true })
	_, err = local.ExecContext(ctx, "LOAD DATA LOCAL INFILE '/etc/hostname' INTO TABLE blobs")
	require.Error(t, err, "LOCAL INFILE stays off")

	tls := appDB(t, sock, func(c *mysql.Config) { c.TLSConfig = "true" })
	assert.Error(t, tls.PingContext(ctx), "TLS is not offered")
}
