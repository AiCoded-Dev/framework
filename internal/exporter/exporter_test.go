package exporter

import (
	"context"
	"errors"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"

	"aicoded.dev/framework/runnerproto/runnerv1"
)

type fakeTelemetry struct {
	mu      sync.Mutex
	spans   int
	dropped uint64
	err     error
}

func (f *fakeTelemetry) Export(_ context.Context, r *connect.Request[runnerv1.ExportRequest]) (*connect.Response[runnerv1.ExportResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	f.spans += len(r.Msg.GetSpans())
	f.dropped += r.Msg.GetDropped()
	return connect.NewResponse(&runnerv1.ExportResponse{}), nil
}

func TestAddNeverBlocks(t *testing.T) {
	e := New(&fakeTelemetry{}, 2)
	for range 1000 {
		e.Add(&runnerv1.Span{})
	}
	assert.Equal(t, uint64(998), e.dropped.Load())
}

func TestRunFlushesOnStop(t *testing.T) {
	f := &fakeTelemetry{}
	e := New(f, 10)
	for range 12 {
		e.Add(&runnerv1.Span{})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e.Run(ctx)
	assert.Equal(t, 10, f.spans)
	assert.Equal(t, uint64(2), f.dropped)
}

func TestFailedExportCountsAsDropped(t *testing.T) {
	e := New(&fakeTelemetry{err: errors.New("runner down")}, 10)
	e.send(context.Background(), []*runnerv1.Span{{}, {}})
	assert.Equal(t, uint64(2), e.dropped.Load())
}
