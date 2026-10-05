package rpc_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/identity"
	"aicoded.dev/framework/rpc"
)

func TestCodeOf(t *testing.T) {
	assert.Equal(t, rpc.Code(0), rpc.CodeOf(nil))
	assert.Equal(t, rpc.NotFound, rpc.CodeOf(rpc.Error(rpc.NotFound, "no invoice 7")))
	assert.Equal(t, rpc.NotFound, rpc.CodeOf(fmt.Errorf("load: %w", rpc.Errorf(rpc.NotFound, "no invoice %d", 7))))
	assert.Equal(t, rpc.Internal, rpc.CodeOf(errors.New("disk full")))
	assert.Equal(t, rpc.Internal, rpc.CodeOf(rpc.Error(0, "no code")))
	assert.Equal(t, rpc.Internal, rpc.CodeOf(rpc.Error(99, "unknown code")))
}

func TestError(t *testing.T) {
	require.EqualError(t, rpc.Errorf(rpc.AlreadyExists, "invoice %d exists", 7), "invoice 7 exists")
	assert.Equal(t, "not_found", rpc.NotFound.String())
	assert.Equal(t, "internal", rpc.Internal.String())
	assert.Equal(t, "code(99)", rpc.Code(99).String())
}

func TestCaller(t *testing.T) {
	assert.Empty(t, rpc.Caller(context.Background()))
	assert.Equal(t, "shop", rpc.Caller(identity.WithCaller(context.Background(), "shop")))
}
