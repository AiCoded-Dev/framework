package web

import (
	"context"
	"log/slog"
	"maps"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"aicoded.dev/framework/internal/redact"
	"aicoded.dev/framework/web/reactive"
)

// Reactive is implemented by the state of a route with live values.
type Reactive interface {
	RouteKey() string
	// Snapshot returns the current value of every live site of the route.
	Snapshot(ctx context.Context) map[string]reactive.Binding
	// Subscribe runs the route's Subscribe hook for as long as the connection lives.
	Subscribe(ctx context.Context, r *Request, conn *reactive.Conn) error
	// HandleWrite applies a value the page wrote to one of the route's variables.
	HandleWrite(ctx context.Context, r *Request, conn *reactive.Conn, msg reactive.WriteMsg)
}

// maxConnAge is how long a live connection lives before the browser must reconnect and pass
// the checks again.
var maxConnAge = 10 * time.Minute

const maxConnsPerViewer = 8

type liveConns struct {
	mu sync.Mutex
	n  map[string]int
}

func newLiveConns() *liveConns { return &liveConns{n: map[string]int{}} }

func (l *liveConns) acquire(subject string) (release func(), ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.n[subject] >= maxConnsPerViewer {
		return nil, false
	}
	l.n[subject]++
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		if l.n[subject]--; l.n[subject] == 0 {
			delete(l.n, subject)
		}
	}, true
}

// serveLive opens the live connection of a page with live values or calls. It passes the same
// checks as the page for every route on the path, runs the Data of the routes that render the
// page, takes their live values, and only then upgrades.
func (m *mux) serveLive(w http.ResponseWriter, r *Request, routes []Route, parent bool) {
	ctx := r.Context()
	if r.Method != http.MethodGet || !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		fail(w, r.Request, Error(http.StatusBadRequest, "This address only takes live page connections."))
		return
	}
	if crossSite(r.Request, true) {
		slog.WarnContext(ctx, "cross-site live connection refused", "origin", r.Header.Get("Origin"), "host", r.Host)
		fail(w, r.Request, Forbidden())
		return
	}
	states, err := admit(ctx, r, routes)
	if err != nil {
		fail(w, r.Request, err)
		return
	}
	if parent {
		fail(w, r.Request, NotFound())
		return
	}
	states = page(routes, states)
	for _, s := range states {
		if err := s.Data(ctx, r, headerWriter{h: http.Header{}}); err != nil {
			fail(w, r.Request, err)
			return
		}
	}
	bindings := map[string]reactive.Binding{}
	byKey := map[string]Reactive{}
	calls := false
	for _, s := range states {
		if l, ok := s.(Reactive); ok {
			maps.Copy(bindings, l.Snapshot(ctx))
			byKey[l.RouteKey()] = l
		}
		if _, ok := s.(Caller); ok {
			calls = true
		}
	}
	if len(byKey) == 0 && !calls {
		fail(w, r.Request, NotFound())
		return
	}
	release, ok := m.live.acquire(r.viewer.Subject)
	if !ok {
		fail(w, r.Request, Error(http.StatusTooManyRequests, "Too many pages are open."))
		return
	}
	defer release()
	age := maxConnAge
	conn, err := reactive.Accept(w, r.Request)
	if err != nil {
		slog.WarnContext(ctx, "live connection not opened", "err", redact.For(ctx, err))
		return
	}
	runLive(ctx, r, conn, routes, byKey, bindings, age)
}

// runLive sends bindings, runs the Subscribe hooks of the routes in byKey, applies writes and
// runs the page's calls until the page goes away, a hook fails or the connection has lived
// for age.
func runLive(ctx context.Context, r *Request, conn *reactive.Conn, routes []Route, byKey map[string]Reactive,
	bindings map[string]reactive.Binding, age time.Duration) {
	defer conn.Close(websocket.StatusNormalClosure, "")
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := func(code websocket.StatusCode, reason string) {
		conn.Close(code, reason)
		cancel()
	}
	expire := time.AfterFunc(age, func() { stop(reactive.StatusReauth, "reconnect") })
	defer expire.Stop()

	if err := conn.Send(ctx, reactive.NewInit(bindings)); err != nil {
		return
	}

	var wg sync.WaitGroup
	wg.Go(func() {
		defer cancel()
		_ = conn.SendLoop(ctx)
	})
	for _, l := range byKey {
		wg.Go(func() {
			defer recoverLive(ctx, stop)
			if err := l.Subscribe(ctx, r, conn); err != nil && ctx.Err() == nil {
				slog.ErrorContext(ctx, "subscribe failed", "err", redact.For(ctx, err))
				stop(websocket.StatusInternalError, "server error")
			}
		})
	}
	inFlight := make(chan struct{}, maxCallsInFlight)
	onCall := func(msg reactive.CallMsg) {
		select {
		case inFlight <- struct{}{}:
		default:
			_ = conn.Send(ctx, reactive.NewFail(msg.ID, http.StatusTooManyRequests, "Too many calls at once."))
			return
		}
		wg.Go(func() {
			defer func() { <-inFlight }()
			runCall(ctx, r, conn, routes, msg)
		})
	}
	wg.Go(func() {
		defer cancel()
		_ = conn.ReadFrames(ctx, func(msg reactive.WriteMsg) {
			defer recoverLive(ctx, stop)
			l, ok := byKey[msg.RouteKey]
			if !ok {
				_ = conn.Send(ctx, reactive.NewErr(msg.RouteKey, msg.Var, "unknown route", reactive.CodeUnknownRoute))
				return
			}
			l.HandleWrite(ctx, r, conn, msg)
		}, onCall)
	})
	wg.Wait()
}

func recoverLive(ctx context.Context, stop func(websocket.StatusCode, string)) {
	if v := recover(); v != nil {
		slog.ErrorContext(ctx, "panic in live page", "value", redact.ValueFor(ctx, v), "stack", string(debug.Stack()))
		stop(websocket.StatusInternalError, "server error")
	}
}
