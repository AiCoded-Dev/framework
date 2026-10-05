package secrets_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/secrets"
)

type fakeSecrets map[string]string

func (f fakeSecrets) GetSecret(_ context.Context, r *connect.Request[runnerv1.GetSecretRequest]) (*connect.Response[runnerv1.GetSecretResponse], error) {
	v, ok := f[r.Msg.GetName()]
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("not declared"))
	}
	return connect.NewResponse(&runnerv1.GetSecretResponse{Value: []byte(v)}), nil
}

var ctx = runner.With(context.Background(), &runner.Session{Secrets: fakeSecrets{"api_key": "s3cret"}})

func TestGet(t *testing.T) {
	v, err := secrets.Get(ctx, "api_key")
	require.NoError(t, err)
	assert.Equal(t, "s3cret", v.Reveal())

	_, err = secrets.Get(ctx, "other")
	assert.Equal(t, "E-MAN-002", errs.Code(err))
	_, err = secrets.Get(context.Background(), "api_key")
	assert.Equal(t, "E-RUN-004", errs.Code(err))
}

func TestValueNeverPrints(t *testing.T) {
	v, err := secrets.Get(ctx, "api_key")
	require.NoError(t, err)

	var logged bytes.Buffer
	slog.New(slog.NewJSONHandler(&logged, nil)).Info("x", "key", v)
	js, err := json.Marshal(struct{ Key secrets.Value }{v})
	require.NoError(t, err)

	for _, s := range []string{
		fmt.Sprint(v),
		fmt.Sprintf("%s %v %+v %#v %q %x", v, v, v, v, v, v),
		logged.String(),
		string(js),
	} {
		assert.NotContains(t, s, "s3cret")
		assert.NotContains(t, s, "733363726574") // hex of s3cret
	}
}

func TestValueNeverPrintsWhenNested(t *testing.T) {
	v, err := secrets.Get(ctx, "api_key")
	require.NoError(t, err)
	type holder struct{ key secrets.Value }
	h := holder{key: v}

	var logged bytes.Buffer
	slog.New(slog.NewTextHandler(&logged, nil)).Info("x", "h", h)
	assert.NotContains(t, logged.String(), "s3cret")
	assert.NotContains(t, logged.String(), "115 51 99 114 101 116") // decimal bytes of s3cret

	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
		out := fmt.Sprintf(verb, h) // verb from a slice, not a constant: vet's printf check cannot analyze it
		assert.NotContains(t, out, "s3cret", "verb %s", verb)
		assert.NotContains(t, out, "115 51 99 114 101 116", "verb %s leaked decimal bytes", verb) // decimal bytes of s3cret
	}
}
