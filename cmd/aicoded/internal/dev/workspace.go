package dev

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/cmd/aicoded/internal/devui"
	"aicoded.dev/framework/cmd/aicoded/internal/devui/deps"
	"aicoded.dev/framework/cmd/aicoded/internal/problem"
	"aicoded.dev/framework/cmd/aicoded/internal/watch"
	"aicoded.dev/framework/cmd/aicoded/internal/workspace"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto/runnerv1"
)

// Config is what Open runs a workspace with.
type Config struct {
	// Root is the workspace's folder and State its private state folder, both absolute.
	Root, State string
	// Dev holds the workspace's settings from dev.yaml. Open reads its port, environment,
	// personas and MySQL server once; port 0 picks a free port, and no environment is dev.
	Dev devconfig.Workspace
	// Values returns the dev.yaml values of app at every start of it; nil uses Dev.Apps.
	Values func(app string) (devconfig.AppValues, error)
	// Out gets the lines aicoded dev prints and the output of the apps.
	Out io.Writer
	// Watch rebuilds an app when its files change, and starts and stops apps as their
	// aicoded.yaml appears and goes.
	Watch bool
	// Manual names the apps the developer runs by hand: aicoded dev keeps their runners up and
	// never builds or starts them.
	Manual []string
	// Login prints the login link of the dev UI, which holds its access token. Without it, the
	// line names only the dev UI's address.
	Login bool
}

// Workspace runs the apps under a folder on the local runner. Each app has a slot that
// generates, builds and starts it; an app that fails stops only itself, and its host shows
// its problems until it starts again.
type Workspace struct {
	cfg     Config
	out     io.Writer
	port    int
	signer  *Signer
	svc     Services
	gateway *Gateway
	server  *http.Server
	ctx     context.Context
	cancel  context.CancelFunc
	exits   sync.WaitGroup
	closing sync.Once

	watcher    *watch.Watcher
	background sync.WaitGroup // the watcher's loop and the checks it starts
	rescanning sync.Mutex
	// unloadable holds the error printed for each new folder whose aicoded.yaml did not load at
	// the last rescan; guarded by rescanning.
	unloadable map[string]string
	mu         sync.Mutex
	slots      []*slot // in folder order
	started    []*slot // in the order of their first start
	personas   bool    // the gateway's personas follow the apps' roles
	changes    *changes

	// control stops the control socket that Host serves; nil when there is none.
	control func()
}

// Open serves the dev UI, then starts the apps under cfg.Root, each after the apps it calls, and
// returns once each is running, runs by hand or has failed. The dev UI's access token is kept in
// cfg.State, which must exist. Only a problem of the whole workspace makes it fail: no app, two
// apps with one name, a name in cfg.Manual that no app has, a token file that is not valid, the
// gateway port in use or a MySQL server dev.yaml cannot name.
func Open(ctx context.Context, cfg Config) (*Workspace, error) {
	if !filepath.IsAbs(cfg.Root) || !filepath.IsAbs(cfg.State) {
		return nil, errors.New("dev: Config.Root and Config.State must be absolute paths")
	}
	apps, err := workspace.Apps(cfg.Root)
	if err != nil {
		return nil, err
	}
	for _, name := range cfg.Manual {
		if !slices.ContainsFunc(apps, func(a workspace.App) bool { return a.Name == name }) {
			return nil, workspace.UnknownApp(name)
		}
	}
	token, err := readToken(cfg.State, true)
	if err != nil {
		return nil, err
	}
	var lc net.ListenConfig
	l, err := lc.Listen(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.Dev.Port)))
	if errors.Is(err, syscall.EADDRINUSE) {
		return nil, errs.New("E-DEV-011", fmt.Sprintf("port %d is in use", cfg.Dev.Port),
			fmt.Sprintf("stop the program that uses port %d, or set port: under workspaces.%s in dev.yaml", cfg.Dev.Port, cfg.Root))
	}
	if err != nil {
		return nil, err
	}
	w, err := newWorkspace(ctx, cfg, apps, l, token)
	if err != nil {
		_ = l.Close()
		return nil, err
	}
	go func() { _ = w.server.Serve(l) }()
	gateway := fmt.Sprintf("http://localhost:%d/", w.port)
	if cfg.Login {
		fmt.Fprintf(w.out, "aicoded dev: the dev UI is at %s\n", loginLink(gateway, token))
	} else {
		fmt.Fprintf(w.out, "aicoded dev: the dev UI is at %s; run aicoded dev in %s to get its login link\n", gateway, cfg.Root)
	}

	start, cancel := context.WithCancel(w.ctx)
	defer cancel()
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	for _, i := range startOrder(w.manifests()) {
		w.start(start, w.slots[i])
	}
	if err := ctx.Err(); err != nil {
		w.Close()
		return nil, err
	}
	if cfg.Watch {
		w.startWatching()
	}
	return w, nil
}

// newWorkspace makes the workspace of apps with the gateway on l and the dev UI for token,
// without starting any app.
func newWorkspace(ctx context.Context, cfg Config, apps []workspace.App, l net.Listener, token string) (*Workspace, error) {
	signer, err := NewSigner()
	if err != nil {
		return nil, err
	}
	w := &Workspace{cfg: cfg, out: &lockedWriter{w: cfg.Out}, port: l.Addr().(*net.TCPAddr).Port, signer: signer, changes: newChanges()}
	w.svc = Services{StateDir: cfg.State, Router: NewRouter(signer), Env: cfg.Dev.Env}
	if cfg.Dev.MySQL != "" {
		if w.svc.MySQL, err = NewMySQLAdmin(cfg.Dev.MySQL); err != nil {
			return nil, err
		}
	}
	for _, a := range apps {
		m, err := manifest.Load(filepath.Join(a.Dir, manifest.FileName))
		if err != nil {
			return nil, err
		}
		s := newSlot(a.Name, a.Dir, m, w.out, w.changes)
		s.manual = slices.Contains(cfg.Manual, a.Name)
		if cfg.Dev.MySQL != "" {
			s.store.hideSecrets(dsnSecrets(cfg.Dev.MySQL))
		}
		w.slots = append(w.slots, s)
	}
	exe, err := os.Executable()
	if err != nil {
		exe = "aicoded"
	}
	w.gateway = NewGateway(w.port, cfg.Dev.Personas, signer, devui.New(w, token, exe, w.out))
	if w.personas = len(cfg.Dev.Personas) == 0; w.personas {
		w.gateway.setPersonas(DefaultPersonas(w.manifests()...))
	}
	w.ctx, w.cancel = context.WithCancel(context.WithoutCancel(ctx))
	w.server = &http.Server{Handler: w.gateway, ReadHeaderTimeout: 10 * time.Second,
		BaseContext: func(net.Listener) context.Context { return w.ctx }, ErrorLog: log.New(w.out, "aicoded dev: ", 0)}
	return w, nil
}

// Close stops the control socket, then watching, then the gateway and then the apps, the last
// started first.
func (w *Workspace) Close() {
	w.closing.Do(func() {
		if w.control != nil {
			w.control()
		}
		// ask starts background work only under w.mu, and only before the cancel.
		w.mu.Lock()
		w.cancel()
		w.mu.Unlock()
		if w.watcher != nil {
			_ = w.watcher.Close()
		}
		w.background.Wait()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = w.server.Shutdown(sctx)
		w.mu.Lock()
		started := slices.Clone(w.started)
		w.mu.Unlock()
		for _, s := range slices.Backward(started) {
			s.cycling.Lock()
			w.stopInstance(s)
			s.cycling.Unlock()
		}
		for _, s := range w.allSlots() {
			s.set(devapi.Stopped, nil)
		}
		w.exits.Wait()
		_ = os.RemoveAll(filepath.Join(w.cfg.State, "bin"))
	})
}

// Status returns the state of every app, in folder order.
func (w *Workspace) Status(context.Context) (devapi.Status, error) {
	st := devapi.Status{Root: w.cfg.Root, Gateway: fmt.Sprintf("http://localhost:%d/", w.port), Apps: []devapi.App{}}
	for _, s := range w.allSlots() {
		st.Apps = append(st.Apps, w.appStatus(s))
	}
	return st, nil
}

// errStopping refuses a request that comes once Close began.
var errStopping = errors.New("aicoded dev is stopping")

// Restart generates, builds and starts the app named app again, also when the developer stopped
// it, and returns its state after. For an app run by hand, it generates the app and keeps its
// runner, unless the runner is down or the permission list or the dev.yaml values changed; then
// it brings up a new runner. It runs on the workspace's context, not on ctx, so only Close ends
// it early.
func (w *Workspace) Restart(_ context.Context, app string) (devapi.App, error) {
	s, err := w.slot(app)
	if err != nil {
		return devapi.App{}, err
	}
	if w.ctx.Err() != nil {
		return devapi.App{}, errStopping
	}
	s.mu.Lock()
	s.stopped = false
	s.mu.Unlock()
	s.cycling.Lock()
	w.runCycle(w.ctx, s, false)
	s.cycling.Unlock()
	return w.appStatus(s), nil
}

// Stop stops the app named app until Start or Restart. The app is not rebuilt when its files
// change. Stop returns at once and stops the app in the background.
func (w *Workspace) Stop(app string) error {
	return w.ask(app, func(s *slot) { s.stopped = true })
}

// Start starts the app named app, or restarts it when it runs; for an app run by hand, it brings
// up a new runner. It returns at once and starts the app in the background.
func (w *Workspace) Start(app string) error {
	return w.ask(app, func(s *slot) { s.stopped, s.forced = false, true })
}

// SetManual switches the app named app into manual mode, where the developer runs it by hand,
// or back. It returns at once and switches in the background; a stopped app stays stopped.
func (w *Workspace) SetManual(app string, on bool) error {
	return w.ask(app, func(s *slot) {
		if s.manual != on {
			s.manual, s.forced = on, true
		}
	})
}

// ask makes change, a request of the developer, to the slot of the app named app, under the
// slot's lock, and applies it in the background, in turn with the cycles of the app. Of
// requests that come while one is applied, the last wins. ask refuses a request once Close began.
func (w *Workspace) ask(app string, change func(*slot)) error {
	s, err := w.slot(app)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ctx.Err() != nil {
		return errStopping
	}
	s.mu.Lock()
	change(s)
	s.mu.Unlock()
	w.kick(s)
	return nil
}

// Personas returns the personas a viewer of the apps can be: those of dev.yaml, or else the
// default ones, which follow the roles of the apps.
func (w *Workspace) Personas() []devconfig.Persona { return w.gateway.Personas() }

// Changed returns a channel that is closed at the next change of the workspace: the state of an
// app, or a log entry, a span or a message that an app sent or received.
func (w *Workspace) Changed() <-chan struct{} { return w.changes.wait() }

// RunWorkspace serves every app under root until ctx is done, with the workspace settings ws
// and the private state folder state, an absolute path. It returns only a problem of the whole
// workspace, as Open does; an app that fails stops only itself.
func RunWorkspace(ctx context.Context, root string, ws devconfig.Workspace, state string, out io.Writer) error {
	w, err := Open(ctx, Config{Root: root, State: state, Dev: ws, Out: out})
	if err != nil {
		return unlessStopped(ctx, err)
	}
	<-ctx.Done()
	w.Close()
	return nil
}

// unlessStopped returns err, or nil when ctx is done because aicoded dev is stopping.
func unlessStopped(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return nil
	}
	return err
}

func (w *Workspace) allSlots() []*slot {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.slots)
}

// slot returns the slot of the app named app, or E-DEV-014.
func (w *Workspace) slot(app string) (*slot, error) {
	for _, s := range w.allSlots() {
		if s.name == app && !s.dup.Load() {
			return s, nil
		}
	}
	return nil, workspace.UnknownApp(app)
}

// names maps the name of every app of the workspace to its folder.
func (w *Workspace) names() map[string]string {
	names := map[string]string{}
	for _, s := range w.allSlots() {
		if !s.dup.Load() {
			names[s.name] = s.dir
		}
	}
	return names
}

// manifests returns the latest permission list of every app, in folder order.
func (w *Workspace) manifests() []manifest.Manifest {
	var ms []manifest.Manifest
	for _, s := range w.allSlots() {
		s.mu.Lock()
		ms = append(ms, s.m)
		s.mu.Unlock()
	}
	return ms
}

func (w *Workspace) appStatus(s *slot) devapi.App {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := devapi.App{Name: s.name, Dir: s.dir, State: s.state, CallsOnly: s.callsOnly, Problems: slices.Clone(s.problems), Since: s.since}
	if !s.callsOnly {
		a.URL = fmt.Sprintf("http://%s.localhost:%d/", s.name, w.port)
	}
	if s.state == devapi.Manual {
		a.Command, a.RunnerDir = manualCommand(s.dir, s.runDir), s.runDir
	}
	return a
}

// values returns the dev.yaml values of app.
func (w *Workspace) values(app string) (devconfig.AppValues, error) {
	if w.cfg.Values != nil {
		return w.cfg.Values(app)
	}
	return w.cfg.Dev.Apps[app], nil
}

var _ deps.Backend = (*Workspace)(nil)

var traceID = regexp.MustCompile(`^[0-9a-f]{32}$`)

// whatBroke is how many error entries and failed spans WhatBroke returns.
const whatBroke = 20

// Logs returns the newest log entries that match q, oldest first.
func (w *Workspace) Logs(_ context.Context, q devapi.LogQuery) ([]devapi.LogEntry, error) {
	limit, err := limitOf(q.Limit, 100, 1000)
	if err != nil {
		return nil, err
	}
	lowest := slog.Level(math.MinInt)
	if q.Level != "" {
		if err := lowest.UnmarshalText([]byte(q.Level)); err != nil {
			return nil, fmt.Errorf("unknown log level %s: use DEBUG, INFO, WARN or ERROR", quote(q.Level))
		}
	}
	slots, err := w.selected(q.App)
	if err != nil {
		return nil, err
	}
	contains := strings.ToLower(q.Contains)
	return newest(logsOf(slots, func(e devapi.LogEntry) bool {
		return levelOf(e) >= lowest && (q.TraceID == "" || e.TraceID == q.TraceID) &&
			strings.Contains(strings.ToLower(e.Message), contains)
	}), limit), nil
}

// Traces returns the newest traces of the app q names, or of every app, newest first.
func (w *Workspace) Traces(_ context.Context, q devapi.TraceQuery) ([]devapi.TraceSummary, error) {
	limit, err := limitOf(q.Limit, 20, 200)
	if err != nil {
		return nil, err
	}
	slots, err := w.selected(q.App)
	if err != nil {
		return nil, err
	}
	byTrace := map[string][]devapi.Span{}
	for _, sp := range spansOf(slots, nil) {
		byTrace[sp.TraceID] = append(byTrace[sp.TraceID], sp)
	}
	out := []devapi.TraceSummary{}
	for id, spans := range byTrace {
		if sum := summarize(id, spans); sum.Error || !q.ErrorsOnly {
			out = append(out, sum)
		}
	}
	slices.SortFunc(out, func(a, b devapi.TraceSummary) int {
		return cmp.Or(b.Start.Compare(a.Start), strings.Compare(a.TraceID, b.TraceID))
	})
	return out[:min(limit, len(out))], nil
}

// Trace returns every span of the trace id, from every app, by start.
func (w *Workspace) Trace(_ context.Context, id string) (devapi.Trace, error) {
	if !traceID.MatchString(id) {
		return devapi.Trace{}, fmt.Errorf("%s is not a trace id of 32 lowercase hex digits", quote(id))
	}
	spans := spansOf(w.allSlots(), func(sp devapi.Span) bool { return sp.TraceID == id })
	if len(spans) == 0 {
		return devapi.Trace{}, fmt.Errorf("no trace %s", id)
	}
	return devapi.Trace{TraceID: id, Spans: spans}, nil
}

// Mail returns the mail app sent, then its mailbox, each newest first.
func (w *Workspace) Mail(_ context.Context, app string) ([]devapi.MailSummary, error) {
	s, err := w.slot(app)
	if err != nil {
		return nil, err
	}
	return s.store.mail.summaries(), nil
}

// MailGet returns the message id that app sent or has in its mailbox.
func (w *Workspace) MailGet(_ context.Context, app, id string) (devapi.Mail, error) {
	s, err := w.slot(app)
	if err != nil {
		return devapi.Mail{}, err
	}
	m, ok := s.store.mail.get(id)
	if !ok {
		return devapi.Mail{}, fmt.Errorf("%s has no message %s", app, quote(id))
	}
	return m, nil
}

// MailReceive puts m into the inbox of app, running or not, and returns its id.
func (w *Workspace) MailReceive(_ context.Context, app string, m devapi.InboundMail) (string, error) {
	s, err := w.slot(app)
	if err != nil {
		return "", err
	}
	if _, err := s.store.mail.rules(); err != nil {
		return "", fmt.Errorf("%s declares no email in aicoded.yaml", app)
	}
	return s.store.mail.deliver(&runnerv1.GetResponse{
		Summary: &runnerv1.MessageSummary{From: &runnerv1.Address{Address: m.From}, Subject: m.Subject},
		Text:    m.Text,
	}), nil
}

// WhatBroke returns the problems of the failed apps and their latest errors and failed spans,
// of app or of every app.
func (w *Workspace) WhatBroke(_ context.Context, app string) (devapi.Broken, error) {
	slots, err := w.selected(app)
	if err != nil {
		return devapi.Broken{}, err
	}
	b := devapi.Broken{Problems: []problem.Problem{}}
	for _, s := range slots {
		if a := w.appStatus(s); a.State == devapi.Failed {
			b.Problems = append(b.Problems, a.Problems...)
		}
	}
	b.Errors = newest(logsOf(slots, func(e devapi.LogEntry) bool { return levelOf(e) >= slog.LevelError }), whatBroke)
	failed := spansOf(slots, func(sp devapi.Span) bool { return sp.Error != "" })
	b.Failed = failed[max(0, len(failed)-whatBroke):]
	return b, nil
}

// selected returns the slot of app, or every slot when app is "".
func (w *Workspace) selected(app string) ([]*slot, error) {
	if app == "" {
		return w.allSlots(), nil
	}
	s, err := w.slot(app)
	if err != nil {
		return nil, err
	}
	return []*slot{s}, nil
}

// limitOf returns n, or def when n is 0; it refuses n outside 1 to most.
func limitOf(n, def, most int) (int, error) {
	if n == 0 {
		return def, nil
	}
	if n < 0 || n > most {
		return 0, fmt.Errorf("limit %d is not between 1 and %d", n, most)
	}
	return n, nil
}

// logsOf returns the log entries of slots for which keep is true, by time.
func logsOf(slots []*slot, keep func(devapi.LogEntry) bool) []devapi.LogEntry {
	out := []devapi.LogEntry{}
	for _, s := range slots {
		for _, e := range s.store.logs.All() {
			if keep(e) {
				out = append(out, e)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b devapi.LogEntry) int { return a.Time.Compare(b.Time) })
	return out
}

// spansOf returns the spans of slots for which keep, when set, is true, by start.
func spansOf(slots []*slot, keep func(devapi.Span) bool) []devapi.Span {
	out := []devapi.Span{}
	for _, s := range slots {
		for _, sp := range s.store.spans.All() {
			if keep == nil || keep(sp) {
				out = append(out, sp)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b devapi.Span) int { return a.Start.Compare(b.Start) })
	return out
}

// newest returns the last n entries of es.
func newest(es []devapi.LogEntry, n int) []devapi.LogEntry {
	return es[max(0, len(es)-n):]
}

// summarize sums up spans, the spans of the trace id. Its root is the earliest span whose parent
// is not in the trace.
func summarize(id string, spans []devapi.Span) devapi.TraceSummary {
	slices.SortStableFunc(spans, func(a, b devapi.Span) int { return a.Start.Compare(b.Start) })
	ids := map[string]bool{}
	for _, sp := range spans {
		ids[sp.SpanID] = true
	}
	sum := devapi.TraceSummary{TraceID: id, Root: spans[0].Name, Apps: []string{}, Spans: len(spans), Start: spans[0].Start}
	if i := slices.IndexFunc(spans, func(sp devapi.Span) bool { return !ids[sp.ParentID] }); i >= 0 {
		sum.Root = spans[i].Name
	}
	end := sum.Start
	for _, sp := range spans {
		if e := sp.Start.Add(time.Duration(sp.DurationMS * float64(time.Millisecond))); e.After(end) {
			end = e
		}
		if !slices.Contains(sum.Apps, sp.App) {
			sum.Apps = append(sum.Apps, sp.App)
		}
		sum.Error = sum.Error || sp.Error != ""
	}
	slices.Sort(sum.Apps)
	sum.DurationMS = float64(end.Sub(sum.Start)) / float64(time.Millisecond)
	return sum
}
