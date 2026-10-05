package pages_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devui/fake"
)

// logged is a workspace of a running app, shop, whose log lines hold markup, and ledger,
// which runs by hand.
func logged() *fake.Backend {
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	return &fake.Backend{Data: fake.Data{
		Status: devapi.Status{Apps: []devapi.App{{Name: "shop", State: devapi.Running}, {Name: "ledger", State: devapi.Manual}}},
		Logs: []devapi.LogEntry{
			{Time: at, App: "shop", Source: "app", Level: "INFO", Message: "first"},
			{
				Time: at.Add(time.Second), App: "shop", Source: "app", Level: "ERROR", Message: "<script>alert(1)</script>",
				TraceID: "0123456789abcdef0123456789abcdef", Attrs: map[string]any{"user": "<b>ana</b>", "n": json.Number("3")},
			},
			{Time: at.Add(2 * time.Second), App: "shop", Source: "runner", Level: "WARN level-ERROR", Message: "odd trace", TraceID: `"><script>`},
		},
	}}
}

func TestLogsPage(t *testing.T) {
	b := logged()
	srv := serve(t, b)
	status, page := get(t, srv, "/logs?app=shop&level=WARN&q=%3Cb%3E&trace=0123456789abcdef0123456789abcdef")
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, []fake.Call{{Method: "Logs", App: "shop", Arg: devapi.LogQuery{
		App: "shop", Level: "WARN", TraceID: "0123456789abcdef0123456789abcdef", Contains: "<b>", Limit: 200,
	}}}, b.Calls("Logs"))
	assert.Contains(t, page, `<a href="/logs">Logs</a>`)
	assert.Contains(t, page, `name="q" value="&lt;b&gt;"`, "the query is shown escaped")
	assert.Contains(t, page, `<option value="ledger">`)
	assert.Contains(t, page, `datetime="2026-09-30T12:00:01Z"`)
	assert.Contains(t, page, `&lt;script&gt;alert(1)&lt;/script&gt;<span class="attrs">n=3 user=&#34;&lt;b&gt;ana&lt;/b&gt;&#34;</span>`)
	assert.Contains(t, page, `<a href="/traces/0123456789abcdef0123456789abcdef">trace</a>`)
	assert.Contains(t, page, `<tr class="level-INFO">`)
	assert.Equal(t, 1, strings.Count(page, "level-ERROR\""), "a level class only for the four levels")
	assert.Contains(t, page, `<tr class="">`)
	assert.Equal(t, 1, strings.Count(page, `<a href="/traces/`), "only a trace id is a link")
	assert.NotContains(t, page, "<script>")
	assert.NotContains(t, page, "<b>")
	assert.NotContains(t, page, "runs by hand", "shop runs under aicoded dev")
	assert.Less(t, strings.Index(page, "odd trace"), strings.Index(page, "alert(1)"), "newest first")
	assert.Less(t, strings.Index(page, "alert(1)"), strings.Index(page, "first"), "newest first")
}

func TestLogsPageNamesAppsRunByHand(t *testing.T) {
	srv := serve(t, logged())
	_, page := get(t, srv, "/logs")
	assert.Contains(t, page, "ledger runs by hand: its log lines go to the terminal it runs in, not here.")
	_, page = get(t, srv, "/logs?app=ledger")
	assert.Contains(t, page, "ledger runs by hand")
}

func TestLogsPageShowsWhyNothingIsListed(t *testing.T) {
	b := logged()
	b.Data.Errors = map[string]error{"Logs": errors.New("unknown log level <b>: use DEBUG, INFO, WARN or ERROR")}
	status, page := get(t, serve(t, b), "/logs?level=%3Cb%3E")
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, page, `<pre class="error">unknown log level &lt;b&gt;: use DEBUG, INFO, WARN or ERROR</pre>`)
	assert.Contains(t, page, "No log lines match.")
}

func TestLogsPageFollowsTheWorkspace(t *testing.T) {
	b := logged()
	srv := serve(t, b)
	_, page := get(t, srv, "/logs?app=shop")
	key := liveKey(t, page, "entries")
	c := live(t, srv, "/logs?app=shop")
	assert.Contains(t, nextPatch(t, c, key), "first")

	b.Update(func(d *fake.Data) {
		d.Logs = append(d.Logs, devapi.LogEntry{Time: time.Now(), App: "shop", Source: "app", Message: "<i>new</i>"})
	})
	assert.Contains(t, nextPatch(t, c, key), "&lt;i&gt;new&lt;/i&gt;")
	calls := b.Calls("Logs")
	assert.Equal(t, "shop", calls[len(calls)-1].App, "the live list keeps the query of its page")
}
