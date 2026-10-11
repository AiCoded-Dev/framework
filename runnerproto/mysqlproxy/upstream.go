package mysqlproxy

import (
	"bytes"
	"context"
	"errors"
	"net"
)

// Upstream is where the proxy opens the app's database sessions: the server and the app's own
// user, password and database.
type Upstream struct {
	Network, Address         string
	User, Password, Database string
}

// clientFlags are the handshake flags the proxy always sends upstream.
const clientFlags = clientLongPassword | clientProtocol41 | clientSecureConnection | clientPluginAuth | clientConnectWithDB

// maxAuthRounds bounds the server's packets in one login. MySQL needs at most an auth switch, a
// request for full authentication and the OK.
const maxAuthRounds = 8

var (
	errSequence = errors.New("mysqlproxy: the server's packets are out of order")
	errRounds   = errors.New("mysqlproxy: too many authentication rounds")
)

// dial connects to the server and reads its greeting. The context's deadline, if it has one,
// stays on the connection.
func dial(ctx context.Context, u Upstream) (net.Conn, *greeting, error) {
	var d net.Dialer
	c, err := d.DialContext(ctx, u.Network, u.Address)
	if err != nil {
		return nil, nil, err
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = c.SetDeadline(dl)
	}
	seq, p, err := readPacket(c, maxHandshake)
	if err == nil && seq != 0 {
		err = errSequence
	}
	if err == nil && len(p) > 0 && p[0] == 0xff {
		err = serverError(p)
	}
	var g *greeting
	if err == nil {
		g, err = parseGreeting(p)
	}
	if err == nil && g.caps&clientFlags != clientFlags {
		err = errors.New("mysqlproxy: the server does not offer the capabilities the runner needs (MySQL 8 is required)")
	}
	if err != nil {
		_ = c.Close()
		return nil, nil, err
	}
	return c, g, nil
}

// login logs in on c as u with the app's session flags, charset and packet size, and returns
// the server's final OK packet. It refuses packets out of order and gives up after
// maxAuthRounds packets from the server.
func login(c net.Conn, g *greeting, u Upstream, flags uint32, charset byte, maxPacket uint32) ([]byte, error) {
	plugin, nonce := g.plugin, g.scramble
	auth, err := scramble(plugin, u.Password, nonce)
	if err != nil {
		return nil, err
	}
	resp := &response{caps: (flags&session | clientFlags) & g.caps, maxPacket: maxPacket, charset: charset,
		user: u.User, auth: auth, db: u.Database, plugin: plugin}
	raw, err := resp.encode()
	if err != nil {
		return nil, err
	}
	s := &sequenced{conn: c}
	if err := s.write(raw); err != nil {
		return nil, err
	}
	for range maxAuthRounds {
		p, err := s.read()
		if err != nil {
			return nil, err
		}
		if len(p) == 0 {
			return nil, errors.New("mysqlproxy: empty packet during login")
		}
		switch {
		case p[0] == 0x00:
			return p, nil
		case p[0] == 0xff:
			return nil, serverError(p)
		case p[0] == 0xfe:
			name, data, ok := cstring(p[1:])
			if !ok {
				return nil, errors.New("mysqlproxy: malformed auth switch")
			}
			plugin, nonce = name, bytes.TrimSuffix(data, []byte{0})
			if auth, err = scramble(plugin, u.Password, nonce); err == nil {
				err = s.write(auth)
			}
		case p[0] == 0x01 && plugin == "caching_sha2_password" && len(p) == 2 && p[1] == 3:
			continue // fast authentication passed; the OK packet follows
		case p[0] == 0x01 && plugin == "caching_sha2_password" && len(p) == 2 && p[1] == 4:
			err = fullAuth(s, u, nonce)
		default:
			return nil, errors.New("mysqlproxy: unexpected packet during login")
		}
		if err != nil {
			return nil, err
		}
	}
	return nil, errRounds
}

// sequenced numbers the packets of one login, which starts after the greeting.
type sequenced struct {
	conn net.Conn
	seq  byte
}

// read reads the server's next packet and refuses one out of order.
func (s *sequenced) read() ([]byte, error) {
	seq, p, err := readPacket(s.conn, maxHandshake)
	if err != nil {
		return nil, err
	}
	if seq != s.seq+1 {
		return nil, errSequence
	}
	s.seq = seq
	return p, nil
}

// write sends the proxy's next packet.
func (s *sequenced) write(p []byte) error {
	s.seq++
	return writePacket(s.conn, s.seq, p)
}

// fullAuth sends the password for caching_sha2_password's full authentication: in clear text
// over a Unix socket, which MySQL counts as secure, and otherwise encrypted with the server's
// RSA key.
func fullAuth(s *sequenced, u Upstream, nonce []byte) error {
	if u.Network == "unix" {
		return s.write(append([]byte(u.Password), 0))
	}
	if err := s.write([]byte{2}); err != nil {
		return err
	}
	p, err := s.read()
	if err != nil {
		return err
	}
	if len(p) > 0 && p[0] == 0xff {
		return serverError(p)
	}
	if len(p) < 2 || p[0] != 0x01 {
		return errors.New("mysqlproxy: the server sent no public key")
	}
	enc, err := encryptPassword(p[1:], u.Password, nonce)
	if err != nil {
		return err
	}
	return s.write(enc)
}
