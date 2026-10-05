package config_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/config"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/runner"
)

func TestString(t *testing.T) {
	ctx := runner.With(context.Background(), &runner.Session{Settings: map[string]string{"greeting": "hi"}})
	v, err := config.String(ctx, "greeting")
	require.NoError(t, err)
	assert.Equal(t, "hi", v)

	_, err = config.String(ctx, "missing")
	assert.Equal(t, "E-MAN-001", errs.Code(err))
	_, err = config.String(context.Background(), "greeting")
	assert.Equal(t, "E-RUN-004", errs.Code(err))
}
