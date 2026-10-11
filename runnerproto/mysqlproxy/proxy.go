package mysqlproxy

import (
	"bufio"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"aicoded.dev/framework/internal/errs"
)

// MaxConns is how many database connections one app may have open through its proxy.
const MaxConns = 32

const (
	handshakeTimeout = 10 * time.Second
	// goodbyeTimeout bounds the last writes to an app whose session ends.
	goodbyeTimeout = time.Second
)

// allowed are the commands an app may send: queries, prepared statements, ping, init-db,
// reset-connection and quit.
var allowed = map[byte]bool{
	0x01: true, // COM_QUIT
	0x02: true, // COM_INIT_DB
	0x03: true, // COM_QUERY
	0x0e: true, // COM_PING
	0x16: true, // COM_STMT_PREPARE
	0x17: true, // COM_STMT_EXECUTE
	0x18: true, // COM_STMT_SEND_LONG_DATA
	0x19: true, // COM_STMT_CLOSE
	0x1a: true, // COM_STMT_RESET
	0x1c: true, // COM_STMT_FETCH
	0x1f: true, // COM_RESET_CONNECTION
}

var errHandshake = errors.New("the app's MySQL client sent a handshake the runner cannot read")

var (
	errUpstream = errPacket(1105, "HY000", errs.New("E-SQL-002", "the runner could not open the app's database",
		"in aicoded dev, check that the MySQL server in dev.yaml runs and read the runner's log line about the database").Error())
	errTooMany = errPacket(1040, "08004", errs.New("E-SQL-004",
		fmt.Sprintf("the app has too many database connections open (at most %d)", MaxConns),
		"close every *sql.Rows and statement, share one *sql.DB from sqldb.Open, and do not raise its pool limit").Error())
	errOrder = errPacket(1156, "08S01", commandError("the app's database packets are out of order"))
)

// clientError is the ERR packet for a client the proxy refuses during the handshake.
func clientError(err error) []byte {
	return errPacket(1251, "08004", errs.New("E-SQL-003", err.Error(),
		"open the database with sqldb.Open, which sets up the client correctly").Error())
}

func commandError(msg string) string {
	return errs.New("E-SQL-001", msg,
		"use the *sql.DB from sqldb.Open for queries and statements; the database user and options are set by the platform").Error()
}

// Proxy serves one app's mysql.sock. It greets the app without asking for credentials, logs in
// upstream as the app's own database user and relays the session.
type Proxy struct {
	// OnError, if set, gets the reason the proxy could not open the app's database. Errors
	// never carry the password. It may be called from several goroutines at once.
	OnError func(error)

	up     Upstream
	slots  chan struct{}
	ids    atomic.Uint32
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup
}

// New returns a proxy that logs in as up.
func New(up Upstream) *Proxy {
	ctx, cancel := context.WithCancel(context.Background())
	return &Proxy{up: up, slots: make(chan struct{}, MaxConns), ctx: ctx, cancel: cancel}
}

// Serve accepts the app's connections on l until l is closed. A connection over MaxConns gets
// E-SQL-004 and is closed.
func (p *Proxy) Serve(l net.Listener) error {
	for {
		c, err := l.Accept()
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		if err != nil {
			return err
		}
		select {
		case p.slots <- struct{}{}:
		default:
			_ = c.SetWriteDeadline(time.Now().Add(goodbyeTimeout))
			_ = writePacket(c, 0, errTooMany)
			_ = c.Close()
			continue
		}
		if !p.start(c) {
			<-p.slots
			_ = c.Close()
		}
	}
}

// start serves c in a goroutine that Close waits for, unless the proxy is closed.
func (p *Proxy) start(c net.Conn) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return false
	}
	p.wg.Go(func() {
		defer func() { <-p.slots }()
		p.handle(c)
	})
	return true
}

// Close ends every open session and waits for them to stop. Close the listener first.
func (p *Proxy) Close() {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	p.cancel()
	p.wg.Wait()
}

func (p *Proxy) fail(err error) {
	if p.OnError != nil && p.ctx.Err() == nil {
		p.OnError(err)
	}
}

func (p *Proxy) handle(app net.Conn) {
	defer app.Close()
	stopApp := context.AfterFunc(p.ctx, func() { _ = app.Close() })
	defer stopApp()
	_ = app.SetDeadline(time.Now().Add(handshakeTimeout))
	ctx, cancel := context.WithTimeout(p.ctx, handshakeTimeout)
	defer cancel()
	up, g, err := dial(ctx, p.up)
	if err != nil {
		p.fail(err)
		_ = writePacket(app, 0, errUpstream)
		return
	}
	defer up.Close()
	stopUp := context.AfterFunc(p.ctx, func() { _ = up.Close() })
	defer stopUp()

	hello := &greeting{version: g.version, connID: p.ids.Add(1), caps: g.caps & offered, charset: g.charset,
		status: g.status, scramble: nonce(), plugin: "mysql_native_password"}
	b, err := hello.encode()
	if err != nil {
		return
	}
	if err := writePacket(app, 0, b); err != nil {
		return
	}
	resp, err := readResponse(app)
	switch {
	case errors.Is(err, errHandshake), errors.Is(err, errTLS), errors.Is(err, errOldClient):
		_ = writePacket(app, 2, clientError(err))
		return
	case err != nil:
		return
	}
	ok, err := login(up, g, p.up, resp.caps&hello.caps, resp.charset, resp.maxPacket)
	if err != nil {
		p.fail(err)
		_ = writePacket(app, 2, errUpstream)
		return
	}
	if err := writePacket(app, 2, ok); err != nil {
		return
	}
	_ = app.SetDeadline(time.Time{})
	_ = up.SetDeadline(time.Time{})
	relay(app, up)
}

// readResponse reads the app's handshake response, which must be packet 1. It returns
// errHandshake, errTLS or errOldClient for a client the proxy refuses.
func readResponse(app io.Reader) (*response, error) {
	seq, raw, err := readPacket(app, maxHandshake)
	switch {
	case errors.Is(err, errTooLarge):
		return nil, errHandshake
	case err != nil:
		return nil, err
	case seq != 1:
		return nil, errHandshake
	}
	resp, err := parseResponse(raw)
	if errors.Is(err, errMalformed) {
		return nil, errHandshake
	}
	return resp, err
}

// nonce returns 20 random bytes in 1..127, like the scramble of MySQL's own greetings.
func nonce() []byte {
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = b[i]%127 + 1
	}
	return b
}

// relay copies the server's packets to the app unchanged and the app's commands to the server
// through forward, until either side ends or forward refuses a command.
func relay(app, up net.Conn) {
	copied := make(chan struct{})
	go func() {
		defer close(copied)
		_, _ = io.Copy(app, up)
		_ = app.SetReadDeadline(time.Now())
	}()
	seq, refusal := forward(app, up)
	_ = up.Close()
	_ = app.SetWriteDeadline(time.Now().Add(goodbyeTimeout))
	<-copied
	if refusal != nil {
		_ = writePacket(app, seq, refusal)
	}
}

// forward copies the app's packets to up. A command must start with sequence id 0 and an
// allowed command byte, and its further packets, when its payload needs more than one, must
// follow in order. Otherwise forward stops and returns the ERR packet for the app with its
// sequence id; it returns a nil packet when either side ends the session.
func forward(app io.Reader, up io.Writer) (byte, []byte) {
	r := bufio.NewReaderSize(app, 64<<10)
	w := bufio.NewWriterSize(up, 64<<10)
	var h [4]byte
	start, next := true, byte(0)
	for {
		if _, err := io.ReadFull(r, h[:]); err != nil {
			return 0, nil
		}
		n, seq := int(h[0])|int(h[1])<<8|int(h[2])<<16, h[3]
		if start {
			cmd, err := r.Peek(min(n, 1))
			if err != nil {
				return 0, nil
			}
			switch {
			case seq != 0:
				return seq + 1, errOrder
			case n == 0:
				return seq + 1, errPacket(1227, "42000", commandError("an empty command is not allowed through the app's database connection"))
			case !allowed[cmd[0]]:
				return seq + 1, errPacket(1227, "42000", commandError(fmt.Sprintf("the command 0x%02x is not allowed through the app's database connection", cmd[0])))
			}
		} else if seq != next {
			return seq + 1, errOrder
		}
		start, next = n < maxPayload, seq+1
		if _, err := w.Write(h[:]); err != nil {
			return 0, nil
		}
		if _, err := io.CopyN(w, r, int64(n)); err != nil {
			return 0, nil
		}
		if err := w.Flush(); err != nil {
			return 0, nil
		}
	}
}
