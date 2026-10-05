package mailer

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/mailrules"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
	"aicoded.dev/framework/runnerproto/socket"
)

// fake is the MailService of an app whose email.to_domains is [acme.example]. Its mailbox holds
// the one message m1. With undeclared set, it answers as for an app without an email section.
type fake struct {
	undeclared atomic.Bool
	mu         sync.Mutex
	requests   []*runnerv1.SendRequest
	list       *runnerv1.ListRequest
	spans      []*runnerv1.Span // exported by the test's goroutine
}

var m1 = &runnerv1.GetResponse{
	Summary: &runnerv1.MessageSummary{
		Id: "m1", From: &runnerv1.Address{Name: "Ana", Address: "ana@acme.example"}, To: []*runnerv1.Address{{Address: "ops@acme.example"}},
		Subject: "Room 12", DateUnix: 1790000000, Size: 42, Folder: Inbox,
	},
	Cc:          []*runnerv1.Address{{Name: "Bo", Address: "bo@acme.example"}},
	ReplyTo:     &runnerv1.Address{Address: "desk@acme.example"},
	Text:        "The tap leaks",
	Html:        "<p>The tap leaks</p>",
	Attachments: []*runnerv1.Attachment{{Filename: "tap.jpg", ContentType: "image/jpeg", Data: []byte{1, 2}}},
	Headers:     []*runnerv1.Header{{Name: "Message-Id", Value: "<t@acme.example>"}},
}

// fakeRunner returns a context whose session talks to a fake over a Unix socket.
func fakeRunner(t *testing.T) (context.Context, *fake) {
	f := &fake{}
	dir, err := os.MkdirTemp("", "aicoded-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, runnerproto.RunnerSocket)
	l, err := socket.Listen(t.Context(), sock)
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.Handle(runnerv1connect.NewMailServiceHandler(f))
	srv := socket.NewServer(mux)
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	client := runnerv1connect.NewMailServiceClient(socket.Client(sock), socket.BaseURL, connect.WithGRPC())
	export := func(s *runnerv1.Span) { f.spans = append(f.spans, s) }
	return runner.With(context.Background(), &runner.Session{Mail: client, Export: export}), f
}

// sent returns the send requests that reached the fake.
func (f *fake) sent() []*runnerv1.SendRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests
}

// listed returns the last list request that reached the fake.
func (f *fake) listed() *runnerv1.ListRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.list
}

func (f *fake) declared() error {
	if f.undeclared.Load() {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("email is not declared in aicoded.yaml"))
	}
	return nil
}

func (f *fake) find(id string) error {
	if err := f.declared(); err != nil || id == "m1" {
		return err
	}
	return connect.NewError(connect.CodeNotFound, errors.New("no such message"))
}

func (f *fake) Send(_ context.Context, r *connect.Request[runnerv1.SendRequest]) (*connect.Response[runnerv1.SendResponse], error) {
	f.mu.Lock()
	f.requests = append(f.requests, r.Msg)
	f.mu.Unlock()
	if err := f.declared(); err != nil {
		return nil, err
	}
	for _, a := range r.Msg.GetTo() {
		if d := mailrules.Domain(a.GetAddress()); d != "acme.example" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errs.New("E-MAIL-003",
				fmt.Sprintf("the domain %q is not in email.to_domains in aicoded.yaml", d), "send only to the domains in email.to_domains"))
		}
	}
	return connect.NewResponse(&runnerv1.SendResponse{Id: "m1"}), nil
}

func (f *fake) List(_ context.Context, r *connect.Request[runnerv1.ListRequest]) (*connect.Response[runnerv1.ListResponse], error) {
	f.mu.Lock()
	f.list = r.Msg
	f.mu.Unlock()
	if err := f.declared(); err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.ListResponse{Messages: []*runnerv1.MessageSummary{m1.GetSummary()}, Total: 1}), nil
}

func (f *fake) Get(_ context.Context, r *connect.Request[runnerv1.GetRequest]) (*connect.Response[runnerv1.GetResponse], error) {
	if err := f.find(r.Msg.GetId()); err != nil {
		return nil, err
	}
	return connect.NewResponse(m1), nil
}

func (f *fake) Delete(_ context.Context, r *connect.Request[runnerv1.DeleteRequest]) (*connect.Response[runnerv1.DeleteResponse], error) {
	return connect.NewResponse(&runnerv1.DeleteResponse{}), f.find(r.Msg.GetId())
}

func (f *fake) Move(_ context.Context, r *connect.Request[runnerv1.MoveRequest]) (*connect.Response[runnerv1.MoveResponse], error) {
	return connect.NewResponse(&runnerv1.MoveResponse{}), f.find(r.Msg.GetId())
}

func TestSend(t *testing.T) {
	ctx, fake := fakeRunner(t)
	id, err := Send(ctx, Message{
		IdempotencyKey: "invoice-42",
		To:             []Address{{Name: "Ana", Address: "ana@acme.example"}},
		Subject:        "Invoice",
		Text:           "Hello",
		Headers:        []Header{{Name: "In-Reply-To", Value: "<a@acme.example>"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "m1", id)
	require.Len(t, fake.sent(), 1)
	assert.Equal(t, "ana@acme.example", fake.sent()[0].GetTo()[0].GetAddress())

	_, err = Send(ctx, Message{To: []Address{{Address: "ana@acme.example"}}, Text: "no key"})
	assert.Equal(t, "E-MAIL-001", errs.Code(err))
	_, err = Send(ctx, Message{IdempotencyKey: "k", To: []Address{{Address: "a@b.example"}}, Subject: "x\r\nBcc: e@evil.example", Text: "t"})
	assert.Equal(t, "E-MAIL-004", errs.Code(err))
	assert.Len(t, fake.sent(), 1, "a message that breaks the rules never reaches the runner")

	fake.undeclared.Store(true)
	_, err = Send(ctx, Message{IdempotencyKey: "k2", To: []Address{{Address: "a@b.example"}}, Text: "t"})
	assert.Equal(t, "E-MAN-012", errs.Code(err))
}

func TestSendRequest(t *testing.T) {
	ctx, fake := fakeRunner(t)
	_, err := Send(ctx, Message{
		IdempotencyKey: "k", FromName: "Ops",
		To: []Address{{Name: "Ana", Address: "ana@acme.example"}}, Cc: []Address{{Address: "bo@acme.example"}}, Bcc: []Address{{Address: "cy@acme.example"}},
		ReplyTo: &Address{Address: "desk@acme.example"}, Subject: "Hi", Text: "t", HTML: "<p>t</p>",
		Attachments: []Attachment{{Filename: "a.pdf", ContentType: "application/pdf", Data: []byte{1}}},
		Headers:     []Header{{Name: "References", Value: "<r@acme.example>"}},
	})
	require.NoError(t, err)
	want := &runnerv1.SendRequest{
		IdempotencyKey: "k", FromName: "Ops",
		To: []*runnerv1.Address{{Name: "Ana", Address: "ana@acme.example"}}, Cc: []*runnerv1.Address{{Address: "bo@acme.example"}}, Bcc: []*runnerv1.Address{{Address: "cy@acme.example"}},
		ReplyTo: &runnerv1.Address{Address: "desk@acme.example"}, Subject: "Hi", Text: "t", Html: "<p>t</p>",
		Attachments: []*runnerv1.Attachment{{Filename: "a.pdf", ContentType: "application/pdf", Data: []byte{1}}},
		Headers:     []*runnerv1.Header{{Name: "References", Value: "<r@acme.example>"}},
	}
	require.Len(t, fake.sent(), 1)
	assert.True(t, proto.Equal(want, fake.sent()[0]), "got %v", fake.sent()[0])
}

func TestRules(t *testing.T) {
	ctx, fake := fakeRunner(t)
	ok := func() Message {
		return Message{IdempotencyKey: "k", To: []Address{{Address: "ana@acme.example"}}, Subject: "Hi", Text: "t"}
	}
	for name, tc := range map[string]struct {
		change func(*Message)
		code   string
	}{
		"no recipients":        {func(m *Message) { m.To = nil }, "E-MAIL-002"},
		"name in address":      {func(m *Message) { m.To[0].Address = "Ana <ana@acme.example>" }, "E-MAIL-002"},
		"two addresses":        {func(m *Message) { m.Cc = []Address{{Address: "a@acme.example, e@evil.example"}} }, "E-MAIL-002"},
		"CRLF in address":      {func(m *Message) { m.Bcc = []Address{{Address: "a@acme.example\r\nBcc: e@evil.example"}} }, "E-MAIL-002"},
		"CRLF in reply-to":     {func(m *Message) { m.ReplyTo = &Address{Address: "a@acme.example\nCc: e@evil.example"} }, "E-MAIL-002"},
		"CRLF in name":         {func(m *Message) { m.To[0].Name = "Ana\r\nBcc: e@evil.example" }, "E-MAIL-004"},
		"CRLF in from name":    {func(m *Message) { m.FromName = "Ops\nBcc: e@evil.example" }, "E-MAIL-004"},
		"CRLF in header value": {func(m *Message) { m.Headers = []Header{{Name: "References", Value: "<a>\r\nBcc: e@evil.example"}} }, "E-MAIL-004"},
		"Bcc header":           {func(m *Message) { m.Headers = []Header{{Name: "Bcc", Value: "e@evil.example"}} }, "E-MAIL-004"},
		"From header":          {func(m *Message) { m.Headers = []Header{{Name: "From", Value: "ceo@acme.example"}} }, "E-MAIL-004"},
		"no body":              {func(m *Message) { m.Text = "" }, "E-MAIL-006"},
		"attachment path":      {func(m *Message) { m.Attachments = []Attachment{{Filename: "../x", ContentType: "text/plain"}} }, "E-MAIL-004"},
	} {
		m := ok()
		tc.change(&m)
		_, err := Send(ctx, m)
		assert.Equal(t, tc.code, errs.Code(err), name)
	}
	assert.Empty(t, fake.sent(), "a message that breaks the rules never reaches the runner")
}

func TestRunnerRefusal(t *testing.T) {
	ctx, fake := fakeRunner(t)
	_, err := Send(ctx, Message{IdempotencyKey: "k", To: []Address{{Address: "eve@evil.example"}}, Subject: "Salaries", Text: "t"})
	assert.Equal(t, "E-MAIL-003", errs.Code(err))
	require.ErrorContains(t, err, `E-MAIL-003: the domain "evil.example" is not in email.to_domains in aicoded.yaml
  fix: send only to the domains in email.to_domains
  docs: `)
	require.Len(t, fake.spans, 1)
	assert.Equal(t, "invalid_argument E-MAIL-003", fake.spans[0].GetError(), "a failed call records only its status and code")
}

func TestUndeclared(t *testing.T) {
	ctx, fake := fakeRunner(t)
	fake.undeclared.Store(true)
	_, _, err := List(ctx, Inbox, 0, 10)
	assert.Equal(t, "E-MAN-012", errs.Code(err))
	_, err = Get(ctx, "m1")
	assert.Equal(t, "E-MAN-012", errs.Code(err))
	assert.Equal(t, "E-MAN-012", errs.Code(Delete(ctx, "m1")))
	assert.Equal(t, "E-MAN-012", errs.Code(Move(ctx, "m1", Archive)))

	_, err = Send(context.Background(), Message{IdempotencyKey: "k", To: []Address{{Address: "ana@acme.example"}}, Text: "t"})
	assert.Equal(t, "E-RUN-004", errs.Code(err))
	_, _, err = List(context.Background(), Inbox, 0, 10)
	assert.Equal(t, "E-RUN-004", errs.Code(err))
	assert.Equal(t, "E-RUN-004", errs.Code(Delete(context.Background(), "m1")))
}

func TestMailbox(t *testing.T) {
	ctx, _ := fakeRunner(t)
	list, total, err := List(ctx, Inbox, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, "m1", list[0].ID)
	m, err := Get(ctx, "m1")
	require.NoError(t, err)
	assert.Equal(t, "m1", m.ID)
	_, err = Get(ctx, "nope")
	require.ErrorIs(t, err, ErrNotFound)
	assert.Equal(t, "E-MAIL-007", errs.Code(Move(ctx, "m1", "spam")))

	_, _, err = List(ctx, "Inbox", 0, 10)
	assert.Equal(t, "E-MAIL-007", errs.Code(err))
	require.NoError(t, Move(ctx, "m1", Trash))
	require.NoError(t, Delete(ctx, "m1"))
	require.ErrorIs(t, Move(ctx, "nope", Archive), ErrNotFound)
	require.ErrorIs(t, Delete(ctx, "nope"), ErrNotFound)
}

func TestListPage(t *testing.T) {
	ctx, fake := fakeRunner(t)
	for _, p := range []struct {
		offset, limit int
		sent          *runnerv1.ListRequest
	}{
		{5, 7, &runnerv1.ListRequest{Folder: Archive, Offset: 5, Limit: 7}},
		{5, math.MaxInt, &runnerv1.ListRequest{Folder: Archive, Offset: 5, Limit: mailrules.MaxList}},
		{-3, 0, &runnerv1.ListRequest{Folder: Archive, Limit: mailrules.DefaultList}},
		{math.MaxInt, -1, &runnerv1.ListRequest{Folder: Archive, Offset: math.MaxInt32, Limit: mailrules.DefaultList}},
	} {
		_, _, err := List(ctx, Archive, p.offset, p.limit)
		require.NoError(t, err)
		assert.True(t, proto.Equal(p.sent, fake.listed()), "List(%d, %d) sent %v", p.offset, p.limit, fake.listed())
	}
}

func TestGet(t *testing.T) {
	ctx, _ := fakeRunner(t)
	m, err := Get(ctx, "m1")
	require.NoError(t, err)
	assert.Equal(t, &Mail{
		Summary: Summary{
			ID: "m1", From: Address{Name: "Ana", Address: "ana@acme.example"}, To: []Address{{Address: "ops@acme.example"}},
			Subject: "Room 12", Date: time.Unix(1790000000, 0), Size: 42, Folder: Inbox,
		},
		Cc:          []Address{{Name: "Bo", Address: "bo@acme.example"}},
		ReplyTo:     &Address{Address: "desk@acme.example"},
		Text:        "The tap leaks",
		HTML:        "<p>The tap leaks</p>",
		Attachments: []Attachment{{Filename: "tap.jpg", ContentType: "image/jpeg", Data: []byte{1, 2}}},
		Headers:     []Header{{Name: "Message-Id", Value: "<t@acme.example>"}},
	}, m)
}

func TestSpans(t *testing.T) {
	ctx, fake := fakeRunner(t)
	_, err := Send(ctx, Message{IdempotencyKey: "k", To: []Address{{Name: "Ana", Address: "ana@acme.example"}}, Subject: "Salaries", Text: "t"})
	require.NoError(t, err)
	_, _, err = List(ctx, Inbox, 0, 10)
	require.NoError(t, err)
	_, err = Get(ctx, "nope")
	require.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, Move(ctx, "m1", Archive))
	require.NoError(t, Delete(ctx, "m1"))

	var names, failures []string
	for _, s := range fake.spans {
		names = append(names, s.GetName())
		failures = append(failures, s.GetError())
		assert.Empty(t, s.GetAttributes(), "a span never holds an address, a subject or an id")
	}
	assert.Equal(t, []string{"mailer.send", "mailer.list", "mailer.get", "mailer.move", "mailer.delete"}, names)
	assert.Equal(t, []string{"", "", "not_found", "", ""}, failures, "a failed call records only its status")
}

func TestMailErr(t *testing.T) {
	e := mailErr(connect.NewError(connect.CodeInvalidArgument, errors.New("E-MAIL-005: the message is too large")))
	want := "E-MAIL-005: the message is too large\n  fix: read the docs page below\n  docs: " + errs.DocsBase + "E-MAIL-005"
	assert.Equal(t, want, e.Error())
	e = mailErr(connect.NewError(connect.CodeInvalidArgument, errors.New("E-MAIL-005: the message is too large\n  docs: https://runner.example/E-MAIL-005")))
	assert.Equal(t, want, e.Error(), "a docs line without a fix line")
	for _, err := range []error{
		connect.NewError(connect.CodeInvalidArgument, errors.New("the request is invalid")),
		connect.NewError(connect.CodeInvalidArgument, errors.New("e-mail-005: lower case")),
		connect.NewError(connect.CodeUnavailable, errors.New("E-MAIL-005: not a refusal")),
	} {
		require.ErrorIs(t, mailErr(err), err)
		assert.Empty(t, errs.Code(mailErr(err)), err.Error())
	}
}
