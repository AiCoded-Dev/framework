package mysqlproxy

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// Capability flags of the MySQL protocol.
const (
	clientLongPassword     uint32 = 1 << 0
	clientFoundRows        uint32 = 1 << 1
	clientLongFlag         uint32 = 1 << 2
	clientConnectWithDB    uint32 = 1 << 3
	clientCompress         uint32 = 1 << 5
	clientLocalFiles       uint32 = 1 << 7
	clientProtocol41       uint32 = 1 << 9
	clientSSL              uint32 = 1 << 11
	clientTransactions     uint32 = 1 << 13
	clientSecureConnection uint32 = 1 << 15
	clientMultiStatements  uint32 = 1 << 16
	clientMultiResults     uint32 = 1 << 17
	clientPSMultiResults   uint32 = 1 << 18
	clientPluginAuth       uint32 = 1 << 19
	clientConnectAttrs     uint32 = 1 << 20
	clientPluginAuthLenenc uint32 = 1 << 21
	clientDeprecateEOF     uint32 = 1 << 24
)

// offered are the capabilities the proxy lets an app use. Compression, TLS, local files,
// multi-statements, connection attributes, session tracking, query attributes and the rest are
// never offered, so the app cannot switch them on.
const offered = clientLongPassword | clientFoundRows | clientLongFlag | clientConnectWithDB | clientProtocol41 |
	clientTransactions | clientSecureConnection | clientMultiResults | clientPSMultiResults | clientPluginAuth |
	clientPluginAuthLenenc | clientDeprecateEOF

// session are the flags that shape the command phase. The upstream session gets exactly the
// app's, so packets pass through unchanged.
const session = clientFoundRows | clientLongFlag | clientProtocol41 | clientTransactions | clientMultiResults |
	clientPSMultiResults | clientDeprecateEOF

var (
	errTLS       = errors.New("the app asked for TLS; the connection to the runner is local and needs none")
	errOldClient = errors.New("the app's MySQL client does not speak protocol 4.1")
	errMalformed = errors.New("mysqlproxy: malformed handshake")
	errEncode    = errors.New("mysqlproxy: the nonce or auth token does not fit the handshake")
)

// greeting is the server's first packet (HandshakeV10).
type greeting struct {
	version  string
	connID   uint32
	caps     uint32
	charset  byte
	status   uint16
	scramble []byte
	plugin   string
}

func parseGreeting(p []byte) (*greeting, error) {
	if len(p) == 0 || p[0] != 10 {
		return nil, errMalformed
	}
	g := &greeting{}
	version, rest, ok := cstring(p[1:])
	if !ok || len(rest) < 4+8+1+2+1+2+2+1+10 {
		return nil, errMalformed
	}
	g.version = version
	g.connID = binary.LittleEndian.Uint32(rest)
	g.scramble = append([]byte{}, rest[4:12]...)
	rest = rest[13:]
	g.caps = uint32(binary.LittleEndian.Uint16(rest))
	g.charset = rest[2]
	g.status = binary.LittleEndian.Uint16(rest[3:])
	g.caps |= uint32(binary.LittleEndian.Uint16(rest[5:])) << 16
	authLen := int(rest[7])
	rest = rest[18:]
	if g.caps&clientSecureConnection != 0 {
		n := max(13, authLen-8)
		if len(rest) < n {
			return nil, errMalformed
		}
		g.scramble = append(g.scramble, bytes.TrimSuffix(rest[:n], []byte{0})...)
		rest = rest[n:]
	}
	if g.caps&clientPluginAuth != 0 {
		if plugin, _, ok := cstring(rest); ok {
			g.plugin = plugin
		} else {
			g.plugin = string(rest)
		}
	}
	return g, nil
}

// encode refuses a scramble shorter than 8 or longer than 254 bytes.
func (g *greeting) encode() ([]byte, error) {
	n := len(g.scramble) + 1
	if n < 9 || n > 0xff {
		return nil, errEncode
	}
	b := append([]byte{10}, g.version...)
	b = append(b, 0)
	b = binary.LittleEndian.AppendUint32(b, g.connID)
	b = append(append(b, g.scramble[:8]...), 0)
	b = binary.LittleEndian.AppendUint16(b, uint16(g.caps&0xffff))
	b = append(b, g.charset)
	b = binary.LittleEndian.AppendUint16(b, g.status)
	b = binary.LittleEndian.AppendUint16(b, uint16(g.caps>>16))
	b = append(b, byte(n))
	b = append(b, make([]byte, 10)...)
	b = append(append(b, g.scramble[8:]...), 0)
	return append(append(b, g.plugin...), 0), nil
}

// response is the client's handshake response (HandshakeResponse41).
type response struct {
	caps      uint32
	maxPacket uint32
	charset   byte
	user      string
	auth      []byte
	db        string
	plugin    string
}

func parseResponse(p []byte) (*response, error) {
	if len(p) < 4 {
		return nil, errMalformed
	}
	r := &response{caps: binary.LittleEndian.Uint32(p)}
	switch {
	case r.caps&clientProtocol41 == 0:
		return nil, errOldClient
	case r.caps&clientSSL != 0:
		return nil, errTLS
	case len(p) < 32:
		return nil, errMalformed
	}
	r.maxPacket = binary.LittleEndian.Uint32(p[4:])
	r.charset = p[8]
	user, rest, ok := cstring(p[32:])
	if !ok {
		return nil, errMalformed
	}
	r.user = user
	switch {
	case r.caps&clientPluginAuthLenenc != 0:
		n, after, ok := lenenc(rest)
		if !ok || uint64(len(after)) < n {
			return nil, errMalformed
		}
		r.auth, rest = after[:n], after[n:]
	case r.caps&clientSecureConnection != 0:
		if len(rest) < 1 || len(rest) < 1+int(rest[0]) {
			return nil, errMalformed
		}
		r.auth, rest = rest[1:1+int(rest[0])], rest[1+int(rest[0]):]
	default:
		auth, after, ok := cstring(rest)
		if !ok {
			return nil, errMalformed
		}
		r.auth, rest = []byte(auth), after
	}
	if r.caps&clientConnectWithDB != 0 && len(rest) > 0 {
		r.db, rest, _ = cstring(rest)
	}
	if r.caps&clientPluginAuth != 0 && len(rest) > 0 {
		r.plugin, _, _ = cstring(rest)
	}
	return r, nil
}

// encode writes the response with a one-byte auth length, so it refuses auth longer than 255
// bytes; the caller sets clientSecureConnection and never clientPluginAuthLenenc.
func (r *response) encode() ([]byte, error) {
	n := len(r.auth)
	if n > 0xff {
		return nil, errEncode
	}
	b := binary.LittleEndian.AppendUint32(nil, r.caps)
	b = binary.LittleEndian.AppendUint32(b, r.maxPacket)
	b = append(b, r.charset)
	b = append(b, make([]byte, 23)...)
	b = append(append(b, r.user...), 0)
	b = append(append(b, byte(n)), r.auth...)
	if r.caps&clientConnectWithDB != 0 {
		b = append(append(b, r.db...), 0)
	}
	if r.caps&clientPluginAuth != 0 {
		b = append(append(b, r.plugin...), 0)
	}
	return b, nil
}
