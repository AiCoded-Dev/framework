package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"slices"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/mcp"
	"aicoded.dev/framework/cmd/aicoded/internal/scaffold"
	"aicoded.dev/framework/cmd/aicoded/internal/testhome"
)

// mcpSession serves aicoded mcp for root over in-memory transports until the test ends.
func mcpSession(t *testing.T, root string) *mcpsdk.ClientSession {
	ct, st := mcpsdk.NewInMemoryTransports()
	done := make(chan error, 1)
	go func() { done <- mcp.Serve(context.Background(), root, st, io.Discard, scaffold.Framework{}) }()
	cs, err := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "e2e", Version: "1"}, nil).Connect(t.Context(), ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = cs.Close()
		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(time.Minute):
			t.Error("aicoded mcp did not stop")
		}
	})
	return cs
}

// callTool calls tool and decodes its structured result into a T.
func callTool[T any](t *testing.T, cs *mcpsdk.ClientSession, tool string, args map[string]any) T {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &mcpsdk.CallToolParams{Name: tool, Arguments: args})
	require.NoError(t, err)
	data, err := json.Marshal(res)
	require.NoError(t, err)
	require.False(t, res.IsError, "%s: %s", tool, data)
	data, err = json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	var v T
	require.NoError(t, json.Unmarshal(data, &v))
	return v
}

// tryTool calls tool and decodes its result. It reports false instead of failing the test, so it
// may run inside Eventually.
func tryTool[T any](ctx context.Context, cs *mcpsdk.ClientSession, tool string, args map[string]any) (T, bool) {
	var v T
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil || res.IsError {
		return v, false
	}
	data, err := json.Marshal(res.StructuredContent)
	return v, err == nil && json.Unmarshal(data, &v) == nil
}

func TestMCPEndToEnd(t *testing.T) {
	requireGo(t)
	home := testhome.Set(t)
	root, err := copyApp("rpc")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	l, err := listenLocal()
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())
	testhome.DevYAML(t, home, fmt.Sprintf("workspaces:\n  %s:\n    port: %d\n", root, port))
	cs := mcpSession(t, root)

	shop := callTool[devapi.App](t, cs, "preview", map[string]any{"app": "shop"})
	require.Equal(t, devapi.Running, shop.State, "%v", shop.Problems)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, shop.URL+"invoices/1", nil)
	require.NoError(t, err)
	resp, body := fetch(t, newClient(port), "all-roles", req)
	require.Equal(t, http.StatusOK, resp.StatusCode, body)
	assert.Contains(t, body, `<h1 id="customer">Hotel Lisboa</h1>`)

	type tracesOut struct {
		Traces []devapi.TraceSummary `json:"traces"`
	}
	var page devapi.Trace
	require.Eventually(t, func() bool {
		traces, _ := tryTool[tracesOut](t.Context(), cs, "traces", map[string]any{})
		for _, tr := range traces.Traces {
			if !slices.Equal(tr.Apps, []string{"billing", "shop"}) {
				continue
			}
			page, _ = tryTool[devapi.Trace](t.Context(), cs, "trace", map[string]any{"trace_id": tr.TraceID})
			if slices.ContainsFunc(page.Spans, func(s devapi.Span) bool { return s.Attrs["rpc.method"] == "GetInvoice" }) {
				return true
			}
		}
		return false
	}, 30*time.Second, 200*time.Millisecond, "the trace of the page spans both apps")
	var apps []string
	for _, s := range page.Spans {
		apps = append(apps, s.App)
	}
	slices.Sort(apps)
	assert.Equal(t, []string{"billing", "shop"}, slices.Compact(apps))

	broken := callTool[devapi.Broken](t, cs, "what_broke", map[string]any{})
	assert.Empty(t, broken.Problems)
	assert.Empty(t, broken.Errors)
	for _, s := range broken.Failed {
		assert.Equal(t, "Secret", s.Attrs["rpc.method"], "only shop's call of Secret as itself fails, in OnStart")
	}
}
