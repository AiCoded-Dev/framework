package dev

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto/runnerv1"
)

const (
	logCount  = 5000
	spanCount = 10000
	mailCount = 1000
	// maxLine is how much output without a newline a lineWriter holds.
	maxLine = 64 << 10
)

// The sources of log entries.
const (
	sourceApp    = "app"
	sourceBuild  = "build"
	sourceRunner = "runner"
)

// store keeps what one app of the workspace logged, traced and mailed, for the life of aicoded
// dev and across the app's restarts. It hides the values of the app's secrets before it keeps
// or prints anything.
type store struct {
	app     string
	out     io.Writer
	logs    *Ring[devapi.LogEntry]
	spans   *Ring[devapi.Span]
	mail    *mailService
	changes *changes // told of every entry and span; may be nil
	mu      sync.Mutex
	hidden  map[string]string // secret value → name, of every start of the app
	hide    *redactor
	limit   *spanLimit // nil: the ring of spans drops its oldest
}

// spanLimit bounds the spans a store keeps: at most count spans, of at most size bytes of text
// in all. The store counts every span past either bound as dropped.
type spanLimit struct {
	count, size int
	kept, bytes int
	dropped     uint64
}

func newStore(app string, out io.Writer) *store {
	s := &store{app: app, out: out, logs: NewRing[devapi.LogEntry](logCount), spans: NewRing[devapi.Span](spanCount), hidden: map[string]string{}}
	s.mail = newMailService(nil, s.redactor)
	return s
}

// limitSpans makes s keep the first spans it gets, at most count spans of at most size bytes of
// text in all, and count the rest as dropped, instead of keeping the last spanCount spans.
func (s *store) limitSpans(count, size int) {
	s.spans = NewRing[devapi.Span](count)
	s.limit = &spanLimit{count: count, size: size}
}

// keepSpan reports whether s keeps sp, and counts it as dropped when not.
func (s *store) keepSpan(sp devapi.Span) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.limit
	if l == nil {
		return true
	}
	n := spanSize(sp)
	if l.kept == l.count || n > l.size-l.bytes {
		l.dropped++
		return false
	}
	l.kept++
	l.bytes += n
	return true
}

// droppedSpans returns how many spans a store with a limit did not keep.
func (s *store) droppedSpans() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.limit == nil {
		return 0
	}
	return s.limit.dropped
}

// spanSize is the length of the text of sp.
func spanSize(sp devapi.Span) int {
	n := len(sp.TraceID) + len(sp.SpanID) + len(sp.ParentID) + len(sp.Name) + len(sp.Error)
	for k, v := range sp.Attrs {
		n += len(k) + len(v)
	}
	return n
}

// hideSecrets hides, from now on, the values of secrets, which maps each secret's name to its
// value. Values of earlier calls stay hidden.
func (s *store) hideSecrets(secrets map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for name, v := range secrets {
		s.hidden[v] = name
	}
	s.hide = newRedactor(s.hidden)
}

// shortSecrets returns the names of the secrets m declares whose values in values are shorter
// than minSecret, so that they are not hidden.
func shortSecrets(m manifest.Manifest, values devconfig.AppValues) []string {
	var short []string
	for _, name := range m.Secrets {
		if len(values.Secrets[name]) < minSecret {
			short = append(short, name)
		}
	}
	return short
}

func (s *store) redactor() *redactor {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hide
}

// write keeps line, a line of output from source, and prints it as "<app> | <line>".
func (s *store) write(source, line string) {
	hide := s.redactor()
	line = hide.String(line)
	e := parseLine(line, time.Now())
	e.App, e.Source = s.app, source
	e.Message, e.TraceID = hide.String(e.Message), hide.String(e.TraceID)
	if e.Attrs != nil {
		e.Attrs = hide.value(e.Attrs).(map[string]any)
	}
	s.logs.Add(e)
	s.changes.signal()
	fmt.Fprintf(s.out, "%s | %s\n", s.app, line)
}

// add keeps an entry that aicoded dev writes about the app, and prints it as "<app> | <msg>"
// when echo is set.
func (s *store) add(source, level, msg string, echo bool) {
	msg = s.redactor().String(msg)
	s.logs.Add(devapi.LogEntry{Time: time.Now(), App: s.app, Source: source, Level: level, Message: msg})
	s.changes.signal()
	if echo {
		fmt.Fprintf(s.out, "%s | %s\n", s.app, msg)
	}
}

// writer returns a writer whose lines write keeps with source.
func (s *store) writer(source string) io.Writer {
	return &lineWriter{
		line: func(line string) { s.write(source, line) },
		cut:  func(text string) int { return s.redactor().cut(text) },
	}
}

// addSpans keeps spans, which the app exported.
func (s *store) addSpans(spans []*runnerv1.Span) {
	hide := s.redactor()
	for _, sp := range spans {
		if span := toSpan(s.app, sp, hide); s.keepSpan(span) {
			s.spans.Add(span)
		}
	}
	s.changes.signal()
}

// parseLine turns line into a log entry at now. A JSON object with a time, a level and a msg,
// as the framework's logger writes, becomes a structured entry: trace_id is its TraceID, span_id
// is dropped and the other keys are its Attrs. Any other line is a plain entry.
func parseLine(line string, now time.Time) devapi.LogEntry {
	plain := devapi.LogEntry{Time: now, Message: line}
	if !strings.HasPrefix(line, "{") {
		return plain
	}
	var rec map[string]any
	dec := json.NewDecoder(strings.NewReader(line))
	dec.UseNumber()
	if err := dec.Decode(&rec); err != nil || dec.More() {
		return plain
	}
	ts, _ := rec["time"].(string)
	level, _ := rec["level"].(string)
	msg, ok := rec["msg"].(string)
	t, err := time.Parse(time.RFC3339Nano, ts)
	var l slog.Level
	if !ok || err != nil || l.UnmarshalText([]byte(level)) != nil {
		return plain
	}
	e := devapi.LogEntry{Time: t, Level: l.String(), Message: msg}
	e.TraceID, _ = rec["trace_id"].(string)
	for _, k := range []string{"time", "level", "msg", "trace_id", "span_id"} {
		delete(rec, k)
	}
	if len(rec) > 0 {
		e.Attrs = rec
	}
	return e
}

// levelOf returns the level of e; a plain line counts as INFO.
func levelOf(e devapi.LogEntry) slog.Level {
	var l slog.Level
	if e.Level == "" || l.UnmarshalText([]byte(e.Level)) != nil {
		return slog.LevelInfo
	}
	return l
}

// toSpan turns sp, a span of app, into the span aicoded dev keeps, with the secret values hidden.
func toSpan(app string, sp *runnerv1.Span, hide *redactor) devapi.Span {
	out := devapi.Span{
		TraceID:    hex.EncodeToString(sp.GetTraceId()),
		SpanID:     hex.EncodeToString(sp.GetSpanId()),
		App:        app,
		Name:       hide.String(sp.GetName()),
		Start:      time.Unix(0, sp.GetStartUnixNano()).UTC(),
		DurationMS: float64(sp.GetEndUnixNano()-sp.GetStartUnixNano()) / float64(time.Millisecond),
		Error:      hide.String(sp.GetError()),
	}
	if p := sp.GetParentSpanId(); len(bytes.Trim(p, "\x00")) > 0 {
		out.ParentID = hex.EncodeToString(p)
	}
	if len(sp.GetAttributes()) > 0 {
		out.Attrs = make(map[string]string, len(sp.GetAttributes()))
		for k, v := range sp.GetAttributes() {
			out.Attrs[hide.String(k)] = hide.String(v)
		}
	}
	return out
}

// lineWriter splits output into lines and hands each to line. Once more than maxLine bytes come
// without a newline, it hands them on as one line too, up to where cut says: the rest may begin a
// secret value, so it waits for the next write.
type lineWriter struct {
	mu   sync.Mutex
	line func(string)
	cut  func(string) int
	buf  []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		n := bytes.IndexByte(w.buf, '\n')
		if n < 0 {
			break
		}
		line := string(w.buf[:n])
		w.buf = w.buf[n+1:]
		w.line(line)
	}
	if len(w.buf) > maxLine {
		if n := w.cut(string(w.buf)); n > 0 {
			w.line(string(w.buf[:n]))
			w.buf = bytes.Clone(w.buf[n:])
		}
	}
	return len(p), nil
}

// lockedWriter serialises writes from several goroutines.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// changes wakes whoever waits for the next change of the workspace.
type changes struct {
	mu   sync.Mutex
	next chan struct{}
}

func newChanges() *changes { return &changes{next: make(chan struct{})} }

// wait returns a channel that is closed at the next change.
func (c *changes) wait() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.next
}

// signal closes the channel of the next change and makes a new one. It does nothing on nil.
func (c *changes) signal() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	close(c.next)
	c.next = make(chan struct{})
}
