// Package exporter batches spans and sends them to the runner without ever blocking the caller.
package exporter

import (
	"context"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"

	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
)

const (
	batchSize    = 256
	interval     = time.Second
	sendTimeout  = 5 * time.Second
	flushTimeout = 2 * time.Second
)

// Exporter queues spans and sends them in batches.
type Exporter struct {
	client  runnerv1connect.TelemetryServiceClient
	queue   chan *runnerv1.Span
	dropped atomic.Uint64
}

// New returns an exporter that queues up to capacity spans.
func New(client runnerv1connect.TelemetryServiceClient, capacity int) *Exporter {
	return &Exporter{client: client, queue: make(chan *runnerv1.Span, capacity)}
}

// Add queues s. When the queue is full, s is dropped and counted.
func (e *Exporter) Add(s *runnerv1.Span) {
	select {
	case e.queue <- s:
	default:
		e.dropped.Add(1)
	}
}

// Run sends batches until ctx is done, then sends what is still queued and returns.
func (e *Exporter) Run(ctx context.Context) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	var batch []*runnerv1.Span
	for {
		select {
		case s := <-e.queue:
			batch = append(batch, s)
			if len(batch) >= batchSize {
				batch = e.send(ctx, batch)
			}
		case <-tick.C:
			batch = e.send(ctx, batch)
		case <-ctx.Done():
			fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), flushTimeout)
			e.send(fctx, e.drain(batch))
			cancel()
			return
		}
	}
}

func (e *Exporter) drain(batch []*runnerv1.Span) []*runnerv1.Span {
	for {
		select {
		case s := <-e.queue:
			batch = append(batch, s)
		default:
			return batch
		}
	}
}

// send exports batch and returns a new empty batch; on failure the spans count as dropped.
func (e *Exporter) send(ctx context.Context, batch []*runnerv1.Span) []*runnerv1.Span {
	dropped := e.dropped.Swap(0)
	if len(batch) == 0 && dropped == 0 {
		return batch
	}
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	if _, err := e.client.Export(ctx, connect.NewRequest(&runnerv1.ExportRequest{Spans: batch, Dropped: dropped})); err != nil {
		e.dropped.Add(dropped)
		for range batch {
			e.dropped.Add(1)
		}
	}
	return nil
}
