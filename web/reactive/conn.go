package reactive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	readLimit   = 64 << 10
	sendTimeout = 10 * time.Second
	writeRate   = 20 // writes per second a page may send
	writeBurst  = 40
)

var (
	errTooManyWrites = errors.New("reactive: too many writes")
	errInvalidFrame  = errors.New("reactive: frame is not a JSON text frame")
)

// Conn is the live connection of one open page.
type Conn struct {
	ws      *websocket.Conn
	mu      sync.Mutex
	patches map[string]*pendingPatch
	notify  chan struct{}
}

type pendingPatch struct {
	b     Binding
	dirty bool
}

// Accept opens a live connection for the request. It clears the read and write deadlines the
// app's HTTP server set, which would otherwise cut the connection after the request timeout.
//
// Generated code only.
func Accept(w http.ResponseWriter, r *http.Request) (*Conn, error) {
	rc := http.NewResponseController(w)
	if err := rc.SetReadDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return nil, err
	}
	if err := rc.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return nil, err
	}
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return nil, err
	}
	ws.SetReadLimit(readLimit)
	return &Conn{ws: ws, patches: map[string]*pendingPatch{}, notify: make(chan struct{}, 1)}, nil
}

// Send writes v as one JSON frame at once. If the client has not taken the frame within
// sendTimeout, Send fails and the connection is closed.
func (c *Conn) Send(ctx context.Context, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	return c.ws.Write(ctx, websocket.MessageText, b)
}

// Enqueue queues b for SendLoop to send as the value of key. A value not sent yet is
// replaced, so only the latest value of a key goes out. Enqueue never blocks.
func (c *Conn) Enqueue(key string, b Binding) {
	c.mu.Lock()
	p, ok := c.patches[key]
	if !ok {
		p = &pendingPatch{}
		c.patches[key] = p
	}
	p.b, p.dirty = b, true
	c.mu.Unlock()
	select {
	case c.notify <- struct{}{}:
	default:
	}
}

// SendLoop sends queued values as patch frames until ctx is done, when it returns nil, or
// until a send fails. A client that does not take a patch within sendTimeout is too slow and
// is cut off.
func (c *Conn) SendLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-c.notify:
		}
		err := c.flush(ctx)
		switch {
		case err == nil:
		case ctx.Err() != nil:
			return nil
		case errors.Is(err, context.DeadlineExceeded):
			c.Close(websocket.StatusPolicyViolation, "slow client")
			return fmt.Errorf("reactive: slow client: %w", err)
		default:
			return err
		}
	}
}

// flush sends every queued value, each as it is at the moment it is sent.
func (c *Conn) flush(ctx context.Context) error {
	c.mu.Lock()
	keys := make([]string, 0, len(c.patches))
	for key, p := range c.patches {
		if p.dirty {
			keys = append(keys, key)
		}
	}
	c.mu.Unlock()
	for _, key := range keys {
		if b, ok := c.take(key); ok {
			if err := c.Send(ctx, PatchMsg{T: "patch", Key: key, Binding: b}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Conn) take(key string) (Binding, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p := c.patches[key]
	dirty := p.dirty
	p.dirty = false
	return p.b, dirty
}

// ReadFrames reads the page's frames until reading fails, and returns that error. Write
// frames go to onWrite and call frames to onCall; a nil handler ignores its frames, and frames
// of any other type are ignored.
//
// Every frame counts against a budget of writeRate frames a second with bursts of writeBurst;
// a page over it is closed with StatusPolicyViolation. A frame that is not a JSON text frame
// closes the connection with StatusUnsupportedData.
func (c *Conn) ReadFrames(ctx context.Context, onWrite func(WriteMsg), onCall func(CallMsg)) error {
	var budget bucket
	for {
		typ, data, err := c.ws.Read(ctx)
		if err != nil {
			return err
		}
		if !budget.take(time.Now()) {
			c.Close(websocket.StatusPolicyViolation, "too many writes")
			return errTooManyWrites
		}
		var f frame
		if typ != websocket.MessageText || json.Unmarshal(data, &f) != nil {
			c.Close(websocket.StatusUnsupportedData, "invalid frame")
			return errInvalidFrame
		}
		switch {
		case f.T == "write" && onWrite != nil:
			onWrite(WriteMsg{T: f.T, RouteKey: f.RouteKey, Var: f.Var, Value: f.Value})
		case f.T == "call" && onCall != nil:
			onCall(CallMsg{T: f.T, ID: f.ID, RouteKey: f.RouteKey, Name: f.Name, Args: f.Args})
		}
	}
}

// frame holds the fields of every frame a page sends.
type frame struct {
	T        string          `json:"t"`
	ID       uint32          `json:"id"`
	RouteKey string          `json:"routeKey"`
	Var      string          `json:"var"`
	Name     string          `json:"name"`
	Value    json.RawMessage `json:"value"`
	Args     json.RawMessage `json:"args"`
}

// Ack accepts a write.
func (c *Conn) Ack(ctx context.Context, msg WriteMsg) {
	_ = c.Send(ctx, AckMsg{T: "ack", RouteKey: msg.RouteKey, Var: msg.Var})
}

// Reject refuses a write with a message and one of the Code constants.
func (c *Conn) Reject(ctx context.Context, msg WriteMsg, text, code string) {
	_ = c.Send(ctx, NewErr(msg.RouteKey, msg.Var, text, code))
}

// Close closes the connection with a status and a reason the browser sees.
func (c *Conn) Close(code websocket.StatusCode, reason string) {
	_ = c.ws.Close(code, reason)
}

type bucket struct {
	tokens float64
	last   time.Time
}

func (b *bucket) take(now time.Time) bool {
	if b.last.IsZero() {
		b.tokens = writeBurst
	} else {
		b.tokens = min(writeBurst, b.tokens+now.Sub(b.last).Seconds()*writeRate)
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
