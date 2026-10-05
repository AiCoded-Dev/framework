package dev

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/manifest"
	"aicoded.dev/framework/cmd/aicoded/internal/problem"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/runnerproto/runnerv1"
)

func TestParseLine(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	at := time.Date(2026, 9, 29, 9, 0, 0, 500, time.UTC)
	for name, c := range map[string]struct {
		line string
		want devapi.LogEntry
	}{
		"slog record": {
			`{"time":"2026-09-29T09:00:00.0000005Z","level":"WARN","msg":"slow","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"00f067aa0ba902b7","ms":12}`,
			devapi.LogEntry{Time: at, Level: "WARN", Message: "slow", TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", Attrs: map[string]any{"ms": json.Number("12")}},
		},
		"a group": {
			`{"time":"2026-09-29T09:00:00.0000005Z","level":"ERROR+2","msg":"x","req":{"path":"/a","ids":[1,2]}}`,
			devapi.LogEntry{Time: at, Level: "ERROR+2", Message: "x", Attrs: map[string]any{"req": map[string]any{"path": "/a", "ids": []any{json.Number("1"), json.Number("2")}}}},
		},
		"plain":               {"listening", devapi.LogEntry{Time: now, Message: "listening"}},
		"JSON without msg":    {`{"time":"2026-09-29T09:00:00Z","level":"INFO"}`, devapi.LogEntry{Time: now, Message: `{"time":"2026-09-29T09:00:00Z","level":"INFO"}`}},
		"not a level":         {`{"time":"2026-09-29T09:00:00Z","level":"LOUD","msg":"x"}`, devapi.LogEntry{Time: now, Message: `{"time":"2026-09-29T09:00:00Z","level":"LOUD","msg":"x"}`}},
		"not a time":          {`{"time":"today","level":"INFO","msg":"x"}`, devapi.LogEntry{Time: now, Message: `{"time":"today","level":"INFO","msg":"x"}`}},
		"more after the JSON": {`{"time":"2026-09-29T09:00:00Z","level":"INFO","msg":"x"} tail`, devapi.LogEntry{Time: now, Message: `{"time":"2026-09-29T09:00:00Z","level":"INFO","msg":"x"} tail`}},
	} {
		assert.Equal(t, c.want, parseLine(c.line, now), name)
	}
}

func TestLogRing(t *testing.T) {
	s := newStore("hello", io.Discard)
	for i := range logCount + 1 {
		s.write(sourceApp, fmt.Sprintf("line %d", i))
	}
	entries := s.logs.All()
	require.Len(t, entries, logCount)
	assert.Equal(t, "line 1", entries[0].Message)
	assert.Equal(t, devapi.LogEntry{Time: entries[0].Time, App: "hello", Source: "app", Message: "line 1"}, entries[0])
}

// testWorkspace returns a workspace of slots for the apps names, which run nothing.
func testWorkspace(names ...string) *Workspace {
	w := &Workspace{port: 8080, out: io.Discard, changes: newChanges(), gateway: NewGateway(8080, nil, nil, nil)}
	w.ctx, w.cancel = context.WithCancel(context.Background())
	for _, n := range names {
		w.slots = append(w.slots, newSlot(n, "/src/"+n, manifest.Manifest{App: n}, io.Discard, w.changes))
	}
	return w
}

func TestChanged(t *testing.T) {
	w := testWorkspace("shop")
	s := w.slots[0]
	s.store.mail.setEmail(acme)
	closed := func(ch <-chan struct{}) bool {
		select {
		case <-ch:
			return true
		default:
			return false
		}
	}
	for name, change := range map[string]func(){
		"a state":        func() { s.set(devapi.Failed, nil) },
		"an app line":    func() { s.store.write(sourceApp, "hi") },
		"a runner entry": func() { s.store.add(sourceRunner, "ERROR", "boom", false) },
		"a span":         func() { s.store.addSpans([]*runnerv1.Span{span("0123456789abcdef", "s1000000", "", "GET", 0, "")}) },
		"a sent mail": func() {
			_, err := s.store.mail.Send(t.Context(), connect.NewRequest(msg("ana@acme.example")))
			require.NoError(t, err)
		},
		"a received mail": func() {
			_, err := w.MailReceive(t.Context(), "shop", devapi.InboundMail{Subject: "hi"})
			require.NoError(t, err)
		},
	} {
		changed := w.Changed()
		require.False(t, closed(changed), name)
		change()
		assert.True(t, closed(changed), name)
		assert.False(t, closed(w.Changed()), "%s: the next change closes a new channel", name)
	}
}

func TestLogsQuery(t *testing.T) {
	w := testWorkspace("billing", "shop")
	billing, shop := w.slots[0].store, w.slots[1].store
	const trace = "4bf92f3577b34da6a3ce929d0e0e4736"
	shop.write(sourceApp, "Plain start")
	billing.write(sourceApp, `{"time":"2020-01-01T00:00:00Z","level":"DEBUG","msg":"detail"}`)
	shop.write(sourceApp, `{"time":"2030-01-01T00:00:00Z","level":"WARN","msg":"slow","trace_id":"`+trace+`"}`)
	billing.add(sourceBuild, "ERROR", "E-CHK-002: undefined: x", false)

	messages := func(q devapi.LogQuery) []string {
		entries, err := w.Logs(t.Context(), q)
		require.NoError(t, err)
		var out []string
		for _, e := range entries {
			out = append(out, e.Message)
		}
		return out
	}
	assert.Equal(t, []string{"detail", "Plain start", "E-CHK-002: undefined: x", "slow"}, messages(devapi.LogQuery{}), "by time over every app")
	assert.Equal(t, []string{"Plain start", "slow"}, messages(devapi.LogQuery{App: "shop"}))
	assert.Equal(t, []string{"Plain start", "E-CHK-002: undefined: x", "slow"}, messages(devapi.LogQuery{Level: "INFO"}), "a plain line counts as INFO")
	assert.Equal(t, []string{"E-CHK-002: undefined: x", "slow"}, messages(devapi.LogQuery{Level: "warn"}))
	assert.Equal(t, []string{"slow"}, messages(devapi.LogQuery{TraceID: trace}))
	assert.Equal(t, []string{"Plain start"}, messages(devapi.LogQuery{Contains: "plain"}))
	assert.Equal(t, []string{"E-CHK-002: undefined: x", "slow"}, messages(devapi.LogQuery{Limit: 2}), "the newest, oldest first")

	_, err := w.Logs(t.Context(), devapi.LogQuery{App: "nope"})
	assert.Equal(t, "E-DEV-014", errs.Code(err))
	_, err = w.Logs(t.Context(), devapi.LogQuery{Level: "LOUD"})
	require.ErrorContains(t, err, `unknown log level "LOUD"`)
	for _, limit := range []int{-1, 1001} {
		_, err = w.Logs(t.Context(), devapi.LogQuery{Limit: limit})
		require.ErrorContains(t, err, "not between 1 and 1000")
	}
}

// span returns a span of trace that starts at ms milliseconds and lasts 10.
func span(trace, id, parent, name string, ms int64, fail string) *runnerv1.Span {
	b := func(s string) []byte {
		if s == "" {
			return nil
		}
		return []byte(s)
	}
	start := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC).Add(time.Duration(ms) * time.Millisecond).UnixNano()
	return &runnerv1.Span{TraceId: b(trace), SpanId: b(id), ParentSpanId: b(parent), Name: name, StartUnixNano: start, EndUnixNano: start + 10e6, Error: fail}
}

func TestTraces(t *testing.T) {
	w := testWorkspace("billing", "shop")
	one, two := "0123456789abcdef", "fedcba9876543210"
	w.slots[1].store.addSpans([]*runnerv1.Span{span(one, "s1000000", "gateway0", "HTTP GET", 0, ""), span(two, "s2000000", "", "HTTP POST", 100, "")})
	w.slots[0].store.addSpans([]*runnerv1.Span{span(one, "b1000000", "s1000000", "GetInvoice", 5, "no invoice")})
	hexOf := func(s string) string { return fmt.Sprintf("%x", s) }

	traces, err := w.Traces(t.Context(), devapi.TraceQuery{})
	require.NoError(t, err)
	require.Len(t, traces, 2)
	assert.Equal(t, hexOf(two), traces[0].TraceID, "newest first")
	assert.Equal(t, devapi.TraceSummary{TraceID: hexOf(one), Root: "HTTP GET", Apps: []string{"billing", "shop"}, Spans: 2,
		Start: traces[1].Start, DurationMS: 15, Error: true}, traces[1], "the root's parent is the gateway, which exports no span")

	only, err := w.Traces(t.Context(), devapi.TraceQuery{ErrorsOnly: true})
	require.NoError(t, err)
	require.Len(t, only, 1)
	assert.Equal(t, hexOf(one), only[0].TraceID)
	limited, err := w.Traces(t.Context(), devapi.TraceQuery{Limit: 1})
	require.NoError(t, err)
	assert.Len(t, limited, 1)
	billing, err := w.Traces(t.Context(), devapi.TraceQuery{App: "billing"})
	require.NoError(t, err)
	require.Len(t, billing, 1)
	assert.Equal(t, []string{"billing"}, billing[0].Apps)
	_, err = w.Traces(t.Context(), devapi.TraceQuery{Limit: 201})
	require.Error(t, err)

	tr, err := w.Trace(t.Context(), hexOf(one))
	require.NoError(t, err)
	require.Len(t, tr.Spans, 2)
	assert.Equal(t, "HTTP GET", tr.Spans[0].Name)
	assert.Equal(t, "billing", tr.Spans[1].App)
	assert.Equal(t, hexOf("s1000000"), tr.Spans[1].ParentID)
	_, err = w.Trace(t.Context(), "0123456789abcdef0123456789abcdef")
	require.ErrorContains(t, err, "no trace 0123456789abcdef0123456789abcdef")
	for _, id := range []string{"xyz", "0123456789ABCDEF0123456789ABCDEF", hexOf(one) + "00"} {
		_, err = w.Trace(t.Context(), id)
		require.ErrorContains(t, err, "is not a trace id", id)
	}
}

func TestMailQueries(t *testing.T) {
	w := testWorkspace("billing", "shop")
	mail := w.slots[1].store.mail
	mail.setEmail(acme)
	for i := range mailCount + 1 {
		m := msg("ana@acme.example")
		m.IdempotencyKey, m.Subject = fmt.Sprint(i), fmt.Sprint("message ", i)
		resp, err := mail.Send(t.Context(), connect.NewRequest(m))
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprint("sent-", i+1), resp.Msg.GetId())
	}
	id, err := w.MailReceive(t.Context(), "shop", devapi.InboundMail{From: "bob@acme.example", Subject: "Hello", Text: "hi"})
	require.NoError(t, err)

	list, err := w.Mail(t.Context(), "shop")
	require.NoError(t, err)
	require.Len(t, list, mailCount+1, "the last 1000 sent, then the inbox")
	assert.Equal(t, "sent-1001", list[0].ID)
	assert.Equal(t, "sent", list[0].Folder)
	assert.Equal(t, "sent-2", list[mailCount-1].ID)
	assert.Equal(t, devapi.MailSummary{ID: id, Folder: "inbox", From: "bob@acme.example", To: []string{}, Subject: "Hello", Time: list[mailCount].Time}, list[mailCount])

	got, err := w.MailGet(t.Context(), "shop", "sent-1001")
	require.NoError(t, err)
	assert.Equal(t, "ops@acme.example", got.From)
	assert.Equal(t, []string{"ana@acme.example"}, got.To)
	assert.Equal(t, "Hello", got.Text)
	inbound, err := w.MailGet(t.Context(), "shop", id)
	require.NoError(t, err)
	assert.Equal(t, "hi", inbound.Text)
	_, err = w.MailGet(t.Context(), "shop", "sent-1")
	require.ErrorContains(t, err, `shop has no message "sent-1"`, "the oldest left the ring")

	_, err = w.MailReceive(t.Context(), "billing", devapi.InboundMail{Subject: "x"})
	require.ErrorContains(t, err, "billing declares no email")
	_, err = w.Mail(t.Context(), "nope")
	assert.Equal(t, "E-DEV-014", errs.Code(err))
	_, err = w.MailReceive(t.Context(), "../x", devapi.InboundMail{})
	assert.Equal(t, "E-DEV-014", errs.Code(err))
}

func TestMailReceiveBeforeStart(t *testing.T) {
	w := &Workspace{slots: []*slot{newSlot("shop", "/src/shop", manifest.Manifest{App: "shop", Email: acme}, io.Discard, nil)}}
	id, err := w.MailReceive(t.Context(), "shop", devapi.InboundMail{Subject: "early"})
	require.NoError(t, err, "an app that never ran takes mail by its permission list")
	got, err := w.MailGet(t.Context(), "shop", id)
	require.NoError(t, err)
	assert.Equal(t, "early", got.Subject)
}

func TestWhatBroke(t *testing.T) {
	w := testWorkspace("billing", "shop")
	failed := problem.Problem{App: "billing", Code: "E-CHK-002", Message: "undefined: x"}
	w.slots[0].set(devapi.Failed, []problem.Problem{failed})
	w.slots[0].store.add(sourceBuild, "ERROR", "E-CHK-002: undefined: x", false)
	w.slots[1].store.write(sourceApp, "fine")
	w.slots[1].store.addSpans([]*runnerv1.Span{span("0123456789abcdef", "s1000000", "", "HTTP GET", 0, "boom"), span("0123456789abcdef", "s2000000", "", "ok", 1, "")})

	b, err := w.WhatBroke(t.Context(), "")
	require.NoError(t, err)
	assert.Equal(t, []problem.Problem{failed}, b.Problems)
	require.Len(t, b.Errors, 1)
	assert.Equal(t, "E-CHK-002: undefined: x", b.Errors[0].Message)
	require.Len(t, b.Failed, 1)
	assert.Equal(t, "boom", b.Failed[0].Error)

	b, err = w.WhatBroke(t.Context(), "shop")
	require.NoError(t, err)
	assert.JSONEq(t, `{"problems":[],"errors":[],"failed_spans":[{"trace_id":"30313233343536373839616263646566",`+
		`"span_id":"7331303030303030","app":"shop","name":"HTTP GET","start":"2026-09-29T09:00:00Z","duration_ms":10,"error":"boom"}]}`,
		jsonOf(t, b), "empty lists, not null")
	_, err = w.WhatBroke(t.Context(), "nope")
	assert.Equal(t, "E-DEV-014", errs.Code(err))
}

func TestLongLineWithoutNewline(t *testing.T) {
	s := newStore("hello", io.Discard)
	w := s.writer(sourceApp).(*lineWriter)
	text := strings.Repeat("0123456789", 10<<10)
	for chunk := range slices.Chunk([]byte(text), 4096) {
		_, err := w.Write(chunk)
		require.NoError(t, err)
		require.LessOrEqual(t, len(w.buf), maxLine)
	}
	fmt.Fprintln(w)
	var got strings.Builder
	var sizes []int
	for _, e := range s.logs.All() {
		got.WriteString(e.Message)
		sizes = append(sizes, len(e.Message))
	}
	assert.Equal(t, []int{17 * 4096, 8 * 4096}, sizes, "what ran past maxLine, then the rest at the newline")
	assert.Equal(t, text, got.String())
}
