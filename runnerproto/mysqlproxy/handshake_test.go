package mysqlproxy

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGreetingRoundTrip(t *testing.T) {
	g := &greeting{version: "8.0.46", connID: 7, caps: offered, charset: 255, status: 2,
		scramble: bytes.Repeat([]byte{'a'}, 20), plugin: "mysql_native_password"}
	b, err := g.encode()
	require.NoError(t, err)
	got, err := parseGreeting(b)
	require.NoError(t, err)
	assert.Equal(t, g, got)
}

func TestEncodeRefuses(t *testing.T) {
	for _, n := range []int{0, 7, 255} {
		_, err := (&greeting{scramble: make([]byte, n)}).encode()
		require.ErrorIs(t, err, errEncode, "a scramble of %d bytes", n)
	}
	_, err := (&response{caps: clientSecureConnection, auth: make([]byte, 256)}).encode()
	assert.ErrorIs(t, err, errEncode, "the auth length is one byte")
}

func TestParseGreetingRefuses(t *testing.T) {
	valid, err := (&greeting{version: "8.0.46", caps: offered, scramble: bytes.Repeat([]byte{'a'}, 20)}).encode()
	require.NoError(t, err)
	for name, p := range map[string][]byte{
		"empty":            nil,
		"protocol 9":       append([]byte{9}, valid[1:]...),
		"no version end":   {10, '8', '.', '0'},
		"truncated":        valid[:20],
		"short auth data":  valid[:len(valid)-8],
		"only the version": []byte("\x0a8.0.46\x00"),
	} {
		_, err := parseGreeting(p)
		assert.ErrorIs(t, err, errMalformed, name)
	}
}

func TestResponseRoundTrip(t *testing.T) {
	r := &response{caps: clientProtocol41 | clientSecureConnection | clientPluginAuth | clientConnectWithDB,
		maxPacket: 1 << 24, charset: 45, user: "app", auth: []byte{1, 2, 3}, db: "notes", plugin: "caching_sha2_password"}
	b, err := r.encode()
	require.NoError(t, err)
	got, err := parseResponse(b)
	require.NoError(t, err)
	assert.Equal(t, r, got)

	lenenc := []byte{}
	lenenc = binary.LittleEndian.AppendUint32(lenenc, clientProtocol41|clientPluginAuthLenenc|clientPluginAuth)
	lenenc = append(lenenc, make([]byte, 28)...)
	lenenc = append(lenenc, "root\x00"...)
	lenenc = append(lenenc, 2, 9, 9)
	lenenc = append(lenenc, "mysql_native_password\x00"...)
	got, err = parseResponse(lenenc)
	require.NoError(t, err)
	assert.Equal(t, "root", got.user)
	assert.Equal(t, []byte{9, 9}, got.auth)
}

func TestParseResponseRefuses(t *testing.T) {
	ssl := binary.LittleEndian.AppendUint32(nil, clientProtocol41|clientSSL)
	ssl = append(ssl, make([]byte, 28)...)
	_, err := parseResponse(ssl)
	require.ErrorIs(t, err, errTLS)

	old := binary.LittleEndian.AppendUint32(nil, clientLongPassword)
	old = append(old, make([]byte, 28)...)
	_, err = parseResponse(old)
	require.ErrorIs(t, err, errOldClient)

	head := append(binary.LittleEndian.AppendUint32(nil, clientProtocol41|clientPluginAuthLenenc), make([]byte, 28)...)
	for _, p := range [][]byte{nil, {1, 2}, binary.LittleEndian.AppendUint32(nil, clientProtocol41),
		append(bytes.Clone(head), "root"...),                     // no end to the user name
		append(bytes.Clone(head), "root\x00\x05ab"...),           // auth shorter than its length
		append(bytes.Clone(head), "root\x00\xfe\xff\xff\xff"...), // truncated length
	} {
		_, err = parseResponse(p)
		assert.ErrorIs(t, err, errMalformed, "%q", p)
	}
}

func TestReadPacketLimit(t *testing.T) {
	var b bytes.Buffer
	require.NoError(t, writePacket(&b, 3, []byte("hello")))
	seq, p, err := readPacket(bytes.NewReader(b.Bytes()), 5)
	require.NoError(t, err)
	assert.Equal(t, byte(3), seq)
	assert.Equal(t, "hello", string(p))
	_, _, err = readPacket(bytes.NewReader(b.Bytes()), 4)
	require.ErrorIs(t, err, errTooLarge, "a handshake packet cannot make the proxy allocate 16 MiB")

	_, _, err = readPacket(bytes.NewReader(b.Bytes()[:7]), 5)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	_, _, err = readPacket(bytes.NewReader(b.Bytes()[:2]), 5)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	assert.Error(t, writePacket(io.Discard, 0, make([]byte, maxPayload)), "a payload of 16 MiB needs a second packet")
}

func TestServerError(t *testing.T) {
	require.EqualError(t, serverError(errPacket(1045, "28000", "Access denied")), "mysql error 1045: Access denied")
	require.EqualError(t, serverError(errPacket(1105, "HY000", strings.Repeat("x", 1000))),
		"mysql error 1105: "+strings.Repeat("x", maxErrorText), "a long message is cut")
	require.Error(t, serverError([]byte{0xff, 1}))
	assert.Error(t, serverError([]byte{0, 1, 2}))
}
