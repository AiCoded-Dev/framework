package mysqlproxy

import (
	"crypto/rsa"
	"crypto/sha1"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/testmysql"
)

func TestLogin(t *testing.T) {
	admin := testmysql.Admin(t)
	cfg := testmysql.Config(t)
	for _, plugin := range []string{"caching_sha2_password", "mysql_native_password"} {
		user, password, db := testmysql.User(t, admin, plugin)
		up := Upstream{Network: cfg.Net, Address: cfg.Addr, User: user, Password: password, Database: db}
		for range 2 { // caching_sha2_password: full authentication first, the fast path second
			c, g, err := dial(t.Context(), up)
			require.NoError(t, err, plugin)
			ok, err := login(c, g, up, clientProtocol41|clientTransactions|clientMultiResults|clientDeprecateEOF, 255, 1<<24)
			require.NoError(t, err, plugin)
			assert.Equal(t, byte(0), ok[0])
			_ = c.Close()
		}
		up.Password = "wrong"
		c, g, err := dial(t.Context(), up)
		require.NoError(t, err)
		_, err = login(c, g, up, clientProtocol41, 255, 1<<24)
		require.ErrorContains(t, err, "1045", plugin)
		_ = c.Close()
	}
}

// sent is a packet the fake server sends, with its sequence id.
type sent struct {
	seq byte
	p   []byte
}

var (
	okPacket   = []byte{0, 0, 0, 2, 0, 0, 0}
	sha2Server = &greeting{caps: ^uint32(0), scramble: testNonce, plugin: "caching_sha2_password"}
)

// fakeLogin logs in as "app" with the password "secret", for an app that asks for every flag, to
// a server that greets with g and then sends packets in order. It returns login's result and the
// payloads the proxy sent.
func fakeLogin(t *testing.T, network string, g *greeting, packets ...sent) ([]byte, [][]byte, error) {
	client, server := net.Pipe()
	require.NoError(t, client.SetDeadline(time.Now().Add(5*time.Second)))
	proxySent := make(chan [][]byte)
	go func() {
		var got [][]byte
		for {
			_, p, err := readPacket(server, maxHandshake)
			if err != nil {
				proxySent <- got
				return
			}
			got = append(got, p)
		}
	}()
	go func() {
		for _, p := range packets {
			if writePacket(server, p.seq, p.p) != nil {
				return
			}
		}
	}()
	up := Upstream{Network: network, User: "app", Password: "secret", Database: "notes"}
	ok, err := login(client, g, up, ^uint32(0), 255, 1<<24)
	_ = client.Close()
	return ok, <-proxySent, err
}

func TestLoginPaths(t *testing.T) {
	ok, out, err := fakeLogin(t, "tcp", sha2Server, sent{2, []byte{1, 3}}, sent{3, okPacket})
	require.NoError(t, err, "fast authentication")
	assert.Equal(t, okPacket, ok)
	require.Len(t, out, 1)
	resp, err := parseResponse(out[0])
	require.NoError(t, err)
	assert.Equal(t, "app", resp.user)
	assert.Equal(t, "notes", resp.db)
	assert.Equal(t, "caching_sha2_password", resp.plugin)
	assert.Equal(t, session|clientFlags, resp.caps, "only the session flags of the app reach the server")

	switchNonce := []byte("abcdefghij0123456789")
	_, out, err = fakeLogin(t, "tcp", sha2Server,
		sent{2, append(append([]byte{0xfe}, "mysql_native_password\x00"...), append(switchNonce, 0)...)}, sent{4, okPacket})
	require.NoError(t, err, "switch to mysql_native_password")
	want, err := scramble("mysql_native_password", "secret", switchNonce)
	require.NoError(t, err)
	require.Len(t, out, 2)
	assert.Equal(t, want, out[1])

	_, out, err = fakeLogin(t, "unix", sha2Server, sent{2, []byte{1, 4}}, sent{4, okPacket})
	require.NoError(t, err, "full authentication over a Unix socket")
	require.Len(t, out, 2)
	assert.Equal(t, []byte("secret\x00"), out[1], "MySQL counts a Unix socket as secure")

	key, pemKey := rsaKey(t)
	_, out, err = fakeLogin(t, "tcp", sha2Server, sent{2, []byte{1, 4}}, sent{4, append([]byte{1}, pemKey...)}, sent{6, okPacket})
	require.NoError(t, err, "full authentication over TCP")
	require.Len(t, out, 3)
	assert.Equal(t, []byte{2}, out[1], "over TCP the proxy asks for the server's key")
	plain, err := rsa.DecryptOAEP(sha1.New(), nil, key, out[2], nil)
	require.NoError(t, err)
	for i := range plain {
		plain[i] ^= testNonce[i%len(testNonce)]
	}
	assert.Equal(t, []byte("secret\x00"), plain)
}

func TestLoginRefuses(t *testing.T) {
	nativeSwitch := append([]byte{0xfe}, "mysql_native_password\x00abcdefghij0123456789\x00"...)
	for name, packets := range map[string][]sent{
		"server error":           {{2, errPacket(1045, "28000", "Access denied")}},
		"clear-text plugin":      {{2, append([]byte{0xfe}, "mysql_clear_password\x00abcdefghij0123456789\x00"...)}},
		"unknown plugin":         {{2, append([]byte{0xfe}, "sha256_password\x00abcdefghij0123456789\x00"...)}},
		"malformed switch":       {{2, append([]byte{0xfe}, "mysql_native_password"...)}},
		"full auth after switch": {{2, nativeSwitch}, {4, []byte{1, 4}}},
		"key that is not PEM":    {{2, []byte{1, 4}}, {4, append([]byte{1}, "not pem"...)}},
		"no key":                 {{2, []byte{1, 4}}, {4, okPacket}},
		"key request refused":    {{2, []byte{1, 4}}, {4, errPacket(1045, "28000", "Access denied")}},
		"unknown auth packet":    {{2, []byte{1, 9}}},
		"empty packet":           {{2, []byte{}}},
		"packet too large":       {{2, make([]byte, maxHandshake+1)}},
		"answer out of order":    {{1, okPacket}},
		"key out of order":       {{2, []byte{1, 4}}, {5, okPacket}},
	} {
		_, out, err := fakeLogin(t, "tcp", sha2Server, packets...)
		require.Error(t, err, name)
		for _, p := range out {
			assert.NotContains(t, string(p), "secret", "%s: the password never leaves in clear text over TCP", name)
		}
	}

	_, _, err := fakeLogin(t, "tcp", sha2Server, sent{3, okPacket})
	require.ErrorIs(t, err, errSequence)
	_, _, err = fakeLogin(t, "tcp", sha2Server, sent{2, []byte{1, 4}}, sent{3, okPacket})
	require.ErrorIs(t, err, errSequence, "the key follows the proxy's request")
}

func TestLoginBoundsRounds(t *testing.T) {
	var fastAuths, fullAuths []sent
	for i := range 3 * maxAuthRounds {
		fastAuths = append(fastAuths, sent{byte(2 + i), []byte{1, 3}})
		fullAuths = append(fullAuths, sent{byte(2 + 2*i), []byte{1, 4}})
	}
	_, _, err := fakeLogin(t, "tcp", sha2Server, fastAuths...)
	require.ErrorIs(t, err, errRounds)
	_, out, err := fakeLogin(t, "unix", sha2Server, fullAuths...)
	require.ErrorIs(t, err, errRounds)
	assert.Len(t, out, 1+maxAuthRounds, "the server cannot make the proxy repeat the password without end")
}

// greeter listens on loopback and sends p with sequence id seq to every connection.
func greeter(t *testing.T, seq byte, p []byte) Upstream {
	var lc net.ListenConfig
	l, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			_ = writePacket(c, seq, p)
			_ = c.Close()
		}
	}()
	return Upstream{Network: "tcp", Address: l.Addr().String()}
}

func TestDialRefuses(t *testing.T) {
	old, err := (&greeting{version: "5.0", caps: clientProtocol41, scramble: testNonce}).encode()
	require.NoError(t, err)
	valid, err := sha2Server.encode()
	require.NoError(t, err)
	_, _, err = dial(t.Context(), greeter(t, 0, valid))
	require.NoError(t, err)
	for name, p := range map[string][]byte{
		"server error":     errPacket(1040, "08004", "Too many connections"),
		"old server":       old,
		"malformed":        {10, '8'},
		"packet too large": make([]byte, maxHandshake+1),
	} {
		c, g, err := dial(t.Context(), greeter(t, 0, p))
		require.Error(t, err, name)
		assert.Nil(t, c, name)
		assert.Nil(t, g, name)
	}
	_, _, err = dial(t.Context(), greeter(t, 1, valid))
	require.ErrorIs(t, err, errSequence, "a greeting is packet 0")
	_, _, err = dial(t.Context(), greeter(t, 0, errPacket(1040, "08004", "Too many connections")))
	assert.EqualError(t, err, "mysql error 1040: Too many connections")
}
