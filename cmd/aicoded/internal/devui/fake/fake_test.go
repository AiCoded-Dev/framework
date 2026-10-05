package fake_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devui/fake"
	"aicoded.dev/framework/internal/errs"
)

func TestBackend(t *testing.T) {
	boom := errors.New("boom")
	b := &fake.Backend{Data: fake.Data{
		Status: devapi.Status{Apps: []devapi.App{{Name: "shop"}}},
		Errors: map[string]error{"Start": boom},
	}}
	require.NoError(t, b.Stop("shop"))
	require.ErrorIs(t, b.Start("shop"), boom)
	assert.Equal(t, "E-DEV-014", errs.Code(b.SetManual("ledger", true)))
	assert.Equal(t, "E-DEV-014", errs.Code(b.Stop("")))
	_, err := b.Logs(t.Context(), devapi.LogQuery{Level: "ERROR"})
	require.NoError(t, err, "a query of every app")
	assert.Equal(t, []fake.Call{
		{Method: "Stop", App: "shop"},
		{Method: "Start", App: "shop"},
		{Method: "SetManual", App: "ledger", Arg: true},
		{Method: "Stop"},
	}, b.Calls("Stop", "Start", "SetManual"))
	assert.Equal(t, []fake.Call{{Method: "Logs", Arg: devapi.LogQuery{Level: "ERROR"}}}, b.Calls("Logs"))

	changed := b.Changed()
	id, err := b.MailReceive(t.Context(), "shop", devapi.InboundMail{From: "a@acme.example", Subject: "Hi", Text: "Hello"})
	require.NoError(t, err)
	select {
	case <-changed:
	default:
		assert.Fail(t, "a received message is a change")
	}
	m, err := b.MailGet(t.Context(), "shop", id)
	require.NoError(t, err)
	assert.Equal(t, "inbox", m.Folder)
	assert.Equal(t, "Hello", m.Text)
}
