// Package mysqlproxy serves an app's mysql.sock. It speaks just enough of the MySQL protocol to
// greet the app without asking for credentials, log in upstream as the app's own database user,
// and relay the session through a command allow-list.
package mysqlproxy

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	// maxHandshake bounds a packet read before the session starts.
	maxHandshake = 64 << 10
	maxPayload   = 1<<24 - 1
	// maxErrorText bounds the server's message that serverError keeps.
	maxErrorText = 512
)

var errTooLarge = errors.New("mysqlproxy: handshake packet too large")

// readPacket reads one packet of at most limit bytes and returns its sequence id and payload.
func readPacket(r io.Reader, limit int) (byte, []byte, error) {
	var h [4]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return 0, nil, err
	}
	n := int(h[0]) | int(h[1])<<8 | int(h[2])<<16
	if n > limit {
		return 0, nil, errTooLarge
	}
	p := make([]byte, n)
	if _, err := io.ReadFull(r, p); err != nil {
		return 0, nil, err
	}
	return h[3], p, nil
}

// writePacket writes payload, shorter than 16 MiB, as one packet.
func writePacket(w io.Writer, seq byte, payload []byte) error {
	n := len(payload)
	if n >= maxPayload {
		return errors.New("mysqlproxy: packet too large")
	}
	b := binary.LittleEndian.AppendUint32(make([]byte, 0, 4+n), uint32(n)|uint32(seq)<<24)
	_, err := w.Write(append(b, payload...))
	return err
}

// errPacket builds an ERR packet.
func errPacket(code uint16, state, msg string) []byte {
	b := binary.LittleEndian.AppendUint16([]byte{0xff}, code)
	b = append(append(b, '#'), state...)
	return append(b, msg...)
}

// serverError turns the server's ERR packet into an error.
func serverError(p []byte) error {
	if len(p) < 3 || p[0] != 0xff {
		return errors.New("mysqlproxy: malformed error packet")
	}
	msg := p[3:]
	if len(msg) >= 6 && msg[0] == '#' {
		msg = msg[6:]
	}
	msg = msg[:min(len(msg), maxErrorText)]
	return fmt.Errorf("mysql error %d: %s", binary.LittleEndian.Uint16(p[1:]), msg)
}

// cstring splits the NUL-terminated string at the start of b from the rest.
func cstring(b []byte) (string, []byte, bool) {
	i := bytes.IndexByte(b, 0)
	if i < 0 {
		return "", nil, false
	}
	return string(b[:i]), b[i+1:], true
}

// lenenc splits a length-encoded integer from the rest of b.
func lenenc(b []byte) (uint64, []byte, bool) {
	if len(b) == 0 {
		return 0, nil, false
	}
	switch b[0] {
	case 0xfc:
		if len(b) < 3 {
			return 0, nil, false
		}
		return uint64(binary.LittleEndian.Uint16(b[1:])), b[3:], true
	case 0xfd:
		if len(b) < 4 {
			return 0, nil, false
		}
		return uint64(b[1]) | uint64(b[2])<<8 | uint64(b[3])<<16, b[4:], true
	case 0xfe:
		if len(b) < 9 {
			return 0, nil, false
		}
		return binary.LittleEndian.Uint64(b[1:]), b[9:], true
	case 0xfb, 0xff:
		return 0, nil, false
	}
	return uint64(b[0]), b[1:], true
}
