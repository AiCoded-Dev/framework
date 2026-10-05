package sqldb

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/runnerproto"
)

func TestOpen(t *testing.T) {
	_, err := Open(context.Background())
	assert.Equal(t, "E-RUN-004", errs.Code(err))

	dir := t.TempDir()
	ctx := runner.With(context.Background(), &runner.Session{App: "notes", Dir: dir})
	_, err = Open(ctx)
	assert.Equal(t, "E-MAN-010", errs.Code(err), "no mysql.sock: the app did not declare sqldb")

	require.NoError(t, os.WriteFile(filepath.Join(dir, runnerproto.MySQLSocket), nil, 0o600))
	db, err := Open(ctx)
	require.NoError(t, err, "Open connects lazily")
	assert.Equal(t, maxOpen, db.Stats().MaxOpenConnections)
	require.NoError(t, db.Close())
}

// The app logs in to mysql.sock with no password, database or TLS: only the runner holds a
// credential.
func TestLoginCarriesNoCredential(t *testing.T) {
	dir := t.TempDir()
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "unix", filepath.Join(dir, runnerproto.MySQLSocket))
	require.NoError(t, err)
	defer ln.Close()
	login := make(chan []byte, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		writePacket(c, 0, greeting())
		login <- readPacket(c)
		writePacket(c, 2, append([]byte{0xff, 0x15, 0x04}, "#28000denied"...))
	}()

	db, err := Open(runner.With(context.Background(), &runner.Session{Dir: dir}))
	require.NoError(t, err)
	defer db.Close()
	var me *mysql.MySQLError
	require.ErrorAs(t, db.PingContext(context.Background()), &me)
	assert.EqualValues(t, 1045, me.Number)

	p := <-login
	require.Greater(t, len(p), 33)
	const withDB, ssl, multiStatements = 1 << 3, 1 << 11, 1 << 16
	assert.Zero(t, binary.LittleEndian.Uint32(p)&(withDB|ssl|multiStatements))
	user, rest, _ := bytes.Cut(p[32:], []byte{0})
	assert.Equal(t, "app", string(user))
	assert.Equal(t, byte(0), rest[0], "an empty auth response")
}

// greeting is a server handshake that offers every capability.
func greeting() []byte {
	b := append([]byte{10}, "8.0.40\x00"...)
	b = append(b, 1, 0, 0, 0)
	b = append(b, "abcdefgh\x00"...)
	b = append(b, 0xff, 0xff, 255, 2, 0, 0xff, 0xff, 21)
	b = append(b, make([]byte, 10)...)
	b = append(b, "ijklmnopqrst\x00"...)
	return append(b, "caching_sha2_password\x00"...)
}

func writePacket(w io.Writer, seq byte, p []byte) {
	_, _ = w.Write(append([]byte{byte(len(p)), byte(len(p) >> 8), byte(len(p) >> 16), seq}, p...))
}

func readPacket(r io.Reader) []byte {
	var h [4]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return nil
	}
	p := make([]byte, int(h[0])|int(h[1])<<8|int(h[2])<<16)
	_, _ = io.ReadFull(r, p)
	return p
}
