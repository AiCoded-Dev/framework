package mysqlproxy

import (
	"bytes"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// shortDir returns a temp dir whose path is short enough for a Unix socket.
func shortDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "mp")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// fake is a MySQL server that greets with every capability, lets anyone in, records what it
// gets and answers every query with OK.
type fake struct {
	sock string

	mu    sync.Mutex
	in    []*response
	cmds  [][]byte
	conns []net.Conn
}

func fakeServer(t *testing.T) *fake {
	f := &fake{sock: filepath.Join(shortDir(t), "db.sock")}
	var lc net.ListenConfig
	l, err := lc.Listen(t.Context(), "unix", f.sock)
	require.NoError(t, err)
	var wg sync.WaitGroup
	t.Cleanup(func() {
		_ = l.Close()
		f.mu.Lock()
		for _, c := range f.conns {
			_ = c.Close()
		}
		f.mu.Unlock()
		wg.Wait()
	})
	wg.Go(func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			f.mu.Lock()
			f.conns = append(f.conns, c)
			f.mu.Unlock()
			wg.Go(func() { f.serve(c) })
		}
	})
	return f
}

func (f *fake) serve(c net.Conn) {
	g := &greeting{version: "8.0.99-fake", connID: 1, caps: 0xffffffff &^ clientPluginAuthLenenc, charset: 255,
		scramble: bytes.Repeat([]byte{'s'}, 20), plugin: "mysql_native_password"}
	b, err := g.encode()
	if err != nil || writePacket(c, 0, b) != nil {
		return
	}
	_, p, err := readPacket(c, maxHandshake)
	if err != nil {
		return
	}
	resp, err := parseResponse(p)
	if err != nil {
		return
	}
	f.mu.Lock()
	f.in = append(f.in, resp)
	f.mu.Unlock()
	if writePacket(c, 2, okPacket) != nil {
		return
	}
	for {
		seq, p, err := readCommand(c)
		if err != nil {
			return
		}
		f.mu.Lock()
		f.cmds = append(f.cmds, p)
		f.mu.Unlock()
		if len(p) > 0 && p[0] == 0x03 && writePacket(c, seq+1, okPacket) != nil {
			return
		}
	}
}

// readCommand reads one payload, which may span several packets, and returns its last
// sequence id.
func readCommand(r io.Reader) (byte, []byte, error) {
	var all []byte
	for {
		seq, p, err := readPacket(r, maxPayload)
		if err != nil {
			return 0, nil, err
		}
		all = append(all, p...)
		if len(p) < maxPayload {
			return seq, all, nil
		}
	}
}

func (f *fake) logins() []*response {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*response{}, f.in...)
}

func (f *fake) packets() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]byte{}, f.cmds...)
}
