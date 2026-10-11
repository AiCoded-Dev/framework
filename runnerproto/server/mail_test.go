package server

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto/mailrules"
	"aicoded.dev/framework/runnerproto/runnerv1"
)

func msg(to string) *runnerv1.SendRequest {
	return &runnerv1.SendRequest{IdempotencyKey: "k-" + to, FromName: "Ops", To: []*runnerv1.Address{{Address: to}}, Subject: "Hi", Text: "Hello"}
}

func TestMailSend(t *testing.T) {
	c, f := serve(t)
	resp, err := c.mail.Send(t.Context(), connect.NewRequest(msg("ana@acme.example")))
	require.NoError(t, err)
	assert.Equal(t, "m-1", resp.Msg.GetId())
	reply := msg("ana@acme.example")
	reply.ReplyTo = &runnerv1.Address{Address: "OPS@acme.example"}
	_, err = c.mail.Send(t.Context(), connect.NewRequest(reply))
	require.NoError(t, err, "Reply-To may be the app's own address")
	assert.Equal(t, []string{"send ops@acme.example", "send ops@acme.example"}, f.mail.seen(), "always from email.from")

	for name, m := range map[string]*runnerv1.SendRequest{
		"foreign domain": msg("eve@evil.example"),
		"subdomain":      msg("ana@mail.acme.example"),
		"foreign cc": func() *runnerv1.SendRequest {
			m := msg("ana@acme.example")
			m.Cc = []*runnerv1.Address{{Address: "eve@evil.example"}}
			return m
		}(),
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
		"a From header": func() *runnerv1.SendRequest {
			m := msg("ana@acme.example")
			m.Headers = []*runnerv1.Header{{Name: "From", Value: "ceo@acme.example"}}
			return m
		}(),
	} {
		_, err := c.mail.Send(t.Context(), connect.NewRequest(m))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), name)
	}
	assert.Len(t, f.mail.seen(), 2, "Mail never saw a refused message")

	var ce *connect.Error
	_, err = c.mail.Send(t.Context(), connect.NewRequest(msg("eve@evil.example")))
	require.ErrorAs(t, err, &ce)
	assert.Contains(t, ce.Message(), "E-MAIL-003")
	assert.Contains(t, ce.Message(), `"evil.example"`)
	assert.NotContains(t, ce.Message(), "eve@", "the refusal names the domain, not the person")
}

func TestMailReplyToTheAppsOwnAddress(t *testing.T) {
	f := newFakes()
	opts := options(t, f)
	opts.Manifest.Email = &manifest.Email{From: "ops@acme.example", ToDomains: []string{"customers.example"}}
	start(t, opts)
	m := msg("ana@customers.example")
	m.ReplyTo = &runnerv1.Address{Address: "ops@acme.example"}
	_, err := dial(opts.Dir).mail.Send(t.Context(), connect.NewRequest(m))
	require.NoError(t, err, "the app's own address, outside email.to_domains")
	m.ReplyTo = &runnerv1.Address{Address: "sales@acme.example"}
	_, err = dial(opts.Dir).mail.Send(t.Context(), connect.NewRequest(m))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "another address of its domain")
}

func TestMailWithoutAnEmailSection(t *testing.T) {
	f := newFakes()
	opts := options(t, f)
	opts.Manifest.Email = nil
	start(t, opts)
	c := dial(opts.Dir)
	_, err := c.mail.Send(t.Context(), connect.NewRequest(msg("ana@acme.example")))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	_, err = c.mail.List(t.Context(), connect.NewRequest(&runnerv1.ListRequest{Folder: "inbox"}))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	_, err = c.mail.Get(t.Context(), connect.NewRequest(&runnerv1.GetRequest{Id: "1"}))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Empty(t, f.mail.seen())
}

func TestMailbox(t *testing.T) {
	c, f := serve(t)
	ctx := t.Context()
	f.mail.page = mailrules.MaxList + 1
	for _, p := range []struct{ offset, limit, wantOffset, wantLimit int32 }{
		{-5, 2, 0, 2},
		{0, 0, 0, mailrules.DefaultList},
		{3, 500, 3, mailrules.MaxList},
	} {
		list, err := c.mail.List(ctx, connect.NewRequest(&runnerv1.ListRequest{Folder: "archive", Offset: p.offset, Limit: p.limit}))
		require.NoError(t, err)
		assert.Len(t, list.Msg.GetMessages(), int(p.wantLimit), "never more than the page")
		assert.Equal(t, int32(1<<31-1), list.Msg.GetTotal(), "the total fits")
		seen := f.mail.seen()
		assert.Equal(t, fmt.Sprintf("list archive %d %d", p.wantOffset, p.wantLimit), seen[len(seen)-1])
	}
	got, err := c.mail.Get(ctx, connect.NewRequest(&runnerv1.GetRequest{Id: "m-1"}))
	require.NoError(t, err)
	assert.Equal(t, "hello", got.Msg.GetText())
	_, err = c.mail.Move(ctx, connect.NewRequest(&runnerv1.MoveRequest{Id: "m-1", Folder: "trash"}))
	require.NoError(t, err)
	_, err = c.mail.Delete(ctx, connect.NewRequest(&runnerv1.DeleteRequest{Id: "m-1"}))
	require.NoError(t, err)
	calls := len(f.mail.seen())

	_, err = c.mail.List(ctx, connect.NewRequest(&runnerv1.ListRequest{Folder: "Inbox"}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = c.mail.Move(ctx, connect.NewRequest(&runnerv1.MoveRequest{Id: "m-1", Folder: "spam"}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	for _, id := range []string{"", "a b", "a\nb", "é", strings.Repeat("x", maxMessageID+1)} {
		_, err = c.mail.Get(ctx, connect.NewRequest(&runnerv1.GetRequest{Id: id}))
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), id)
		_, err = c.mail.Delete(ctx, connect.NewRequest(&runnerv1.DeleteRequest{Id: id}))
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), id)
		_, err = c.mail.Move(ctx, connect.NewRequest(&runnerv1.MoveRequest{Id: id, Folder: "inbox"}))
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), id)
	}
	assert.Len(t, f.mail.seen(), calls, "Mail never saw a refused call")
}

func TestMailProviderErrors(t *testing.T) {
	c, f := serve(t)
	f.mail.fail(connect.NewError(connect.CodeUnimplemented, errors.New("E-MAIL-005: this target keeps no inbox")))
	_, err := c.mail.List(t.Context(), connect.NewRequest(&runnerv1.ListRequest{Folder: "inbox"}))
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
	require.ErrorContains(t, err, "E-MAIL-005", "a target's refusal reaches the app")

	f.mail.fail(errors.New("smtp 10.0.0.9:587: 535 bad password for ops@acme.example"))
	_, err = c.mail.Send(t.Context(), connect.NewRequest(msg("ana@acme.example")))
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	assert.NotContains(t, err.Error(), "10.0.0.9")
	assert.NotContains(t, err.Error(), "password")
}
