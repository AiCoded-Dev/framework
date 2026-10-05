package pages_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devui/fake"
)

const traceID = "0123456789abcdef0123456789abcdef"

// traced is a workspace with one trace, through shop and billing, whose names, attributes and
// errors hold markup. A child of its first child starts after its second child, and two of its
// spans are each other's parents.
func traced() *fake.Backend {
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	span := func(id, parent, app, name string, start, ms float64) devapi.Span {
		return devapi.Span{
			TraceID: traceID, SpanID: id, ParentID: parent, App: app, Name: name,
			Start: at.Add(time.Duration(start * float64(time.Millisecond))), DurationMS: ms,
		}
	}
	charge := span("c", "a", "billing", "<script>charge</script>", 4, 8)
	charge.Attrs, charge.Error = map[string]string{"amount": "<i>9</i>"}, "<b>declined</b>"
	return &fake.Backend{Data: fake.Data{
		Status: devapi.Status{Apps: []devapi.App{{Name: "shop"}, {Name: "billing"}}},
		Traces: []devapi.TraceSummary{{
			TraceID: traceID, Root: "GET <b>/</b>", Apps: []string{"billing", "shop"}, Spans: 6, Start: at, DurationMS: 12.5, Error: true,
		}},
		Spans: []devapi.Span{
			span("a", "", "shop", "GET /", 0, 12.5),
			span("b", "a", "shop", "query", 1, 5),
			charge,
			span("d", "b", "shop", "insert", 5, 1),
			span("e", "f", "shop", "looped", 6, 0.5),
			span("f", "e", "shop", "looped back", 6.5, 0.5),
		},
	}}
}

func TestTracesPage(t *testing.T) {
	b := traced()
	srv := serve(t, b)
	status, page := get(t, srv, "/traces?app=shop&errors=1")
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, []fake.Call{{Method: "Traces", App: "shop", Arg: devapi.TraceQuery{App: "shop", ErrorsOnly: true, Limit: 100}}}, b.Calls("Traces"))
	assert.Contains(t, page, `value="1" checked>`)
	assert.Contains(t, page, `<a href="/traces/`+traceID+`">GET &lt;b&gt;/&lt;/b&gt;</a></td><td>billing, shop</td><td>6</td><td>12.5ms</td><td><span class="error">error</span>`)
	assert.NotContains(t, page, "<b>")

	_, page = get(t, srv, "/traces")
	assert.Contains(t, page, `value="1"> Only with an error`)
	assert.NotContains(t, page, "checked")
}

func TestTracesPageFollowsTheWorkspace(t *testing.T) {
	b := traced()
	srv := serve(t, b)
	_, page := get(t, srv, "/traces")
	key := liveKey(t, page, "traces")
	c := live(t, srv, "/traces")
	assert.Contains(t, nextPatch(t, c, key), "GET &lt;b&gt;/&lt;/b&gt;")

	b.Update(func(d *fake.Data) {
		d.Traces = append([]devapi.TraceSummary{{TraceID: strings.Repeat("f", 32), Root: "<i>POST</i>", Spans: 1}}, d.Traces...)
	})
	assert.Contains(t, nextPatch(t, c, key), "&lt;i&gt;POST&lt;/i&gt;")
}

func TestTracePage(t *testing.T) {
	srv := serve(t, traced())
	status, page := get(t, srv, "/traces/"+traceID)
	require.Equal(t, http.StatusOK, status)
	assert.Contains(t, page, "<p>12.5ms in billing, shop. ")
	assert.Contains(t, page, `<a href="/logs?trace=`+traceID+`">`)
	last := -1
	for _, row := range []string{
		`<td class="depth-0">GET /</td>`,
		`<td class="depth-1">query</td>`,
		`<td class="depth-2">insert</td>`,
		`<td class="depth-1">&lt;script&gt;charge&lt;/script&gt;<ul class="attrs"><li>amount=&lt;i&gt;9&lt;/i&gt;</li></ul></td>`,
		`<td class="depth-0">looped</td>`,
		`<td class="depth-1">looped back</td>`,
	} {
		i := strings.Index(page, row)
		assert.Greater(t, i, last, "%s: each span follows its parent, and a loop of parents comes last", row)
		last = i
	}
	assert.Contains(t, page, `<td>billing</td><td>+4ms</td><td>8ms</td><td><meter min="0" max="12.5" value="8"></meter></td><td class="error">&lt;b&gt;declined&lt;/b&gt;</td>`)
	assert.NotContains(t, page, "<script>")
	assert.NotContains(t, page, "<b>")
}

func TestTracePageNeedsATraceID(t *testing.T) {
	b := traced()
	srv := serve(t, b)
	for _, id := range []string{"xyz", strings.ToUpper(traceID), traceID + "0", traceID[1:]} {
		status, _ := get(t, srv, "/traces/"+id)
		assert.Equal(t, http.StatusNotFound, status, id)
	}
	assert.Empty(t, b.Calls("Trace"), "the page asks only for a trace id")

	status, _ := get(t, srv, "/traces/"+strings.Repeat("f", 32))
	assert.Equal(t, http.StatusNotFound, status, "a trace aicoded dev does not keep")
}
