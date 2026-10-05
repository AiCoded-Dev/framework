package dev

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/manifest"
	"aicoded.dev/framework/runnerproto/mailrules"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
)

func mailClient(t *testing.T, email *manifest.Email) (runnerv1connect.MailServiceClient, *mailService) {
	svc := newMailService(email, nil)
	mux := http.NewServeMux()
	mux.Handle(runnerv1connect.NewMailServiceHandler(svc))
	return runnerv1connect.NewMailServiceClient(serveSocket(t, mux), socketBaseURL, connect.WithGRPC()), svc
}

var acme = &manifest.Email{From: "ops@acme.example", ToDomains: []string{"acme.example"}}

func msg(to string) *runnerv1.SendRequest {
	return &runnerv1.SendRequest{IdempotencyKey: "k-" + to, FromName: "Ops", To: []*runnerv1.Address{{Address: to}}, Subject: "Hi", Text: "Hello"}
}

func TestMailSend(t *testing.T) {
	c, svc := mailClient(t, acme)
	resp, err := c.Send(t.Context(), connect.NewRequest(msg("ana@acme.example")))
	require.NoError(t, err)
	again, err := c.Send(t.Context(), connect.NewRequest(msg("ana@acme.example")))
	require.NoError(t, err)
	assert.Equal(t, resp.Msg.GetId(), again.Msg.GetId(), "the same idempotency key sends once")
	sent := svc.sentMail()
	require.Len(t, sent, 1)
	assert.Equal(t, "ops@acme.example", sent[0].GetSummary().GetFrom().GetAddress(), "the sender is always email.from")
	assert.Equal(t, "Ops", sent[0].GetSummary().GetFrom().GetName())

	for name, m := range map[string]*runnerv1.SendRequest{
		"foreign domain": msg("eve@evil.example"),
		"subdomain":      msg("ana@mail.acme.example"),
		"foreign bcc": func() *runnerv1.SendRequest {
			m := msg("ana@acme.example")
			m.Bcc = []*runnerv1.Address{{Address: "eve@evil.example"}}
			return m
		}(),
		"foreign reply": func() *runnerv1.SendRequest {
			m := msg("ana@acme.example")
			m.ReplyTo = &runnerv1.Address{Address: "eve@evil.example"}
			return m
		}(),
		"broken rule": func() *runnerv1.SendRequest {
			m := msg("ana@acme.example")
			m.Subject = "a\r\nBcc: eve@evil.example"
			return m
		}(),
	} {
		_, err := c.Send(t.Context(), connect.NewRequest(m))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), name)
	}
	var ce *connect.Error
	_, err = c.Send(t.Context(), connect.NewRequest(msg("eve@evil.example")))
	require.ErrorAs(t, err, &ce)
	assert.Contains(t, ce.Message(), "E-MAIL-003")
	assert.Contains(t, ce.Message(), `"evil.example"`)
	assert.NotContains(t, ce.Message(), "eve@", "the refusal names the domain, not the person")
	long := strings.Repeat("d", 63) + "." + strings.Repeat("e", 63) + ".example"
	m := msg("ana@acme.example")
	m.Cc = []*runnerv1.Address{{Address: "eve@" + long}}
	_, err = c.Send(t.Context(), connect.NewRequest(m))
	require.ErrorAs(t, err, &ce)
	assert.Contains(t, ce.Message(), `"`+long[:64]+`"...`, "a long domain is cut")
	m.Cc, m.IdempotencyKey, m.ReplyTo = nil, "reply", &runnerv1.Address{Address: "ops@acme.example"}
	_, err = c.Send(t.Context(), connect.NewRequest(m))
	require.NoError(t, err, "Reply-To may be the app's own address")
	assert.Len(t, svc.sentMail(), 2)

	none, _ := mailClient(t, nil)
	_, err = none.Send(t.Context(), connect.NewRequest(msg("ana@acme.example")))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err), "an app without an email section")
}

func TestMailbox(t *testing.T) {
	c, svc := mailClient(t, acme)
	first := svc.deliver(&runnerv1.GetResponse{Summary: &runnerv1.MessageSummary{Subject: "one"}, Text: "1"})
	second := svc.deliver(&runnerv1.GetResponse{Summary: &runnerv1.MessageSummary{Subject: "two"}, Text: "2"})

	list, err := c.List(t.Context(), connect.NewRequest(&runnerv1.ListRequest{Folder: "inbox"}))
	require.NoError(t, err)
	assert.Equal(t, int32(2), list.Msg.GetTotal())
	assert.Equal(t, second, list.Msg.GetMessages()[0].GetId(), "newest first")

	_, err = c.Move(t.Context(), connect.NewRequest(&runnerv1.MoveRequest{Id: first, Folder: "archive"}))
	require.NoError(t, err)
	_, err = c.Move(t.Context(), connect.NewRequest(&runnerv1.MoveRequest{Id: first, Folder: "spam"}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	got, err := c.Get(t.Context(), connect.NewRequest(&runnerv1.GetRequest{Id: first}))
	require.NoError(t, err)
	assert.Equal(t, "archive", got.Msg.GetSummary().GetFolder())
	assert.Equal(t, "1", got.Msg.GetText())

	_, err = c.Delete(t.Context(), connect.NewRequest(&runnerv1.DeleteRequest{Id: first}))
	require.NoError(t, err)
	_, err = c.Get(t.Context(), connect.NewRequest(&runnerv1.GetRequest{Id: first}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	_, err = c.List(t.Context(), connect.NewRequest(&runnerv1.ListRequest{Folder: "Inbox"}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestMailPaging(t *testing.T) {
	c, svc := mailClient(t, acme)
	ids := make([]string, 120) // newest first
	for i := range ids {
		ids[len(ids)-1-i] = svc.deliver(&runnerv1.GetResponse{Text: strconv.Itoa(i)})
	}
	for _, p := range []struct {
		offset, limit int32
		want          []string
	}{
		{1, 1, ids[1:2]},
		{110, 20, ids[110:]},
		{120, 10, nil},
		{math.MaxInt32, math.MaxInt32, nil},
		{-5, 2, ids[:2]},
		{0, 0, ids[:mailrules.DefaultList]},
		{0, 500, ids[:mailrules.MaxList]},
	} {
		list, err := c.List(t.Context(), connect.NewRequest(&runnerv1.ListRequest{Folder: "inbox", Offset: p.offset, Limit: p.limit}))
		require.NoError(t, err)
		var got []string
		for _, m := range list.Msg.GetMessages() {
			got = append(got, m.GetId())
		}
		assert.Equal(t, p.want, got, "offset %d, limit %d", p.offset, p.limit)
		assert.Equal(t, int32(len(ids)), list.Msg.GetTotal())
	}
}
