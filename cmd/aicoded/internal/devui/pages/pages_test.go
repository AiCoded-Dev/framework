package pages_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/cmd/aicoded/internal/devui/deps"
	"aicoded.dev/framework/cmd/aicoded/internal/devui/fake"
	"aicoded.dev/framework/cmd/aicoded/internal/devui/pages"
	"aicoded.dev/framework/cmd/aicoded/internal/problem"
	"aicoded.dev/framework/internal/identity"
)

// since is when the apps of apps entered their state, on a day that is not today.
var since = time.Date(2025, 9, 30, 14, 5, 9, 0, time.UTC)

// apps is a workspace of two apps whose state, URL and problem hold markup; the problem's docs
// link runs a script.
func apps() *fake.Backend {
	return &fake.Backend{Data: fake.Data{Status: devapi.Status{Apps: []devapi.App{
		{Name: "shop", State: "<b>running</b>", URL: `http://shop.localhost:8080/"><script>`, Since: since, Problems: []problem.Problem{{
			Code: "E-GEN-030", Pos: "pages/index.html:3", Message: "<script>alert(1)</script>", Fix: "add <ssr:access/>", Docs: "javascript:alert(1)",
		}}},
		{Name: "billing", State: devapi.Stopped, CallsOnly: true, Since: since},
	}}}}
}

func TestAppsPage(t *testing.T) {
	srv := serve(t, apps())
	status, page := get(t, srv, "/")
	assert.Equal(t, http.StatusFound, status)
	assert.Contains(t, page, `<a href="/apps">`, "the root opens the apps")

	status, page = get(t, srv, "/apps")
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, page, "<title>aicoded dev</title>")
	assert.Contains(t, page, `<a href="/apps">Apps</a>`)
	when := `<td><time datetime="2025-09-30T14:05:09Z">` + since.Local().Format(time.DateTime) + `</time></td>`
	assert.Contains(t, page, `<tr> <td><a href="/apps/shop">shop</a></td> <td>&lt;b&gt;running&lt;/b&gt;</td> `+when+` `+
		`<td><a href="http://shop.localhost:8080/%22%3e%3cscript%3e">http://shop.localhost:8080/&#34;&gt;&lt;script&gt;</a></td> <td>E-GEN-030</td> </tr>`)
	assert.Contains(t, page, `<tr> <td><a href="/apps/billing">billing</a></td> <td>stopped</td> `+when+` <td></td> <td></td> </tr>`)
	assert.NotContains(t, page, "<b>")
	assert.NotContains(t, page, "<script>")
}

func TestAppsPageFollowsTheWorkspace(t *testing.T) {
	b := apps()
	srv := serve(t, b)
	_, page := get(t, srv, "/apps")
	key := liveKey(t, page, "apps")
	c := live(t, srv, "/apps")
	assert.Contains(t, nextPatch(t, c, key), "<td>&lt;b&gt;running&lt;/b&gt;</td>")

	b.Update(func(d *fake.Data) { d.Status.Apps[0].State = devapi.Failed })
	assert.Contains(t, nextPatch(t, c, key), `<a href="/apps/shop">shop</a></td> <td>failed</td>`)
}

// The pages refuse a request without the developer, although the front never sends one.
func TestPagesNeedTheDeveloper(t *testing.T) {
	h := pages.NewHandler(deps.New(apps(), executable))
	for want, v := range map[int]identity.Identity{
		http.StatusUnauthorized: {},
		http.StatusForbidden:    {Subject: "alice", Name: "Alice", Roles: []string{"admin"}},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequestWithContext(as(t.Context(), v), http.MethodGet, "/apps", nil))
		assert.Equal(t, want, w.Code)
		assert.NotContains(t, w.Body.String(), "shop")
	}
}

func TestAppPage(t *testing.T) {
	srv := serve(t, apps())
	status, page := get(t, srv, "/apps/shop")
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, page, "<h1>shop</h1>")
	assert.Equal(t, "<b>running</b>", elementText(t, page, "state"))
	assert.Contains(t, page, `<time datetime="2025-09-30T14:05:09Z">`)
	assert.Equal(t, since.Local().Format(time.DateTime), elementText(t, page, "since"))
	assert.Contains(t, page, "<strong>E-GEN-030</strong> <code>pages/index.html:3</code>")
	assert.Contains(t, page, "<pre>&lt;script&gt;alert(1)&lt;/script&gt;</pre>")
	assert.Contains(t, page, "Fix: add &lt;ssr:access/&gt;")
	assert.Contains(t, page, `<a href="about:invalid#aicoded-unsafe-url">javascript:alert(1)</a>`)
	assert.NotContains(t, page, "<b>")
	assert.NotContains(t, page, "<script>")
	assert.NotContains(t, page, "<h2>Run it yourself</h2>", "the command shows only in manual mode")
}

func TestAppPageInManualMode(t *testing.T) {
	b := apps()
	b.Data.Status.Apps[1] = devapi.App{Name: "billing", State: devapi.Manual, CallsOnly: true,
		Command: "cd /w/billing && AICODED_RUNNER_DIR=/tmp/run go run .", RunnerDir: "/tmp/run"}
	_, page := get(t, serve(t, b), "/apps/billing")
	assert.Equal(t, "manual", elementText(t, page, "state"))
	assert.Equal(t, "The app has no pages.", elementText(t, page, "address"))
	assert.Contains(t, page, "<pre>cd /w/billing &amp;&amp; AICODED_RUNNER_DIR=/tmp/run go run .</pre>")
	assert.Contains(t, page, "<pre>/tmp/run</pre>")
}

func TestAppPageFollowsTheWorkspace(t *testing.T) {
	b := apps()
	srv := serve(t, b)
	_, page := get(t, srv, "/apps/shop")
	stateKey, manualKey := liveKey(t, page, "state"), liveKey(t, page, "manual")
	state, manual := live(t, srv, "/apps/shop"), live(t, srv, "/apps/shop")
	assert.Equal(t, "<b>running</b>", nextPatch(t, state, stateKey))
	assert.Empty(t, nextPatch(t, manual, manualKey))

	b.Update(func(d *fake.Data) {
		d.Status.Apps[0].State = devapi.Manual
		d.Status.Apps[0].Command = "cd /w/shop && AICODED_RUNNER_DIR=/tmp/run go run ."
	})
	assert.Equal(t, "manual", nextPatch(t, state, stateKey))
	assert.Contains(t, nextPatch(t, manual, manualKey), "<pre>cd /w/shop &amp;&amp; AICODED_RUNNER_DIR=/tmp/run go run .</pre>")
}

func TestAppForms(t *testing.T) {
	for form, want := range map[string]fake.Call{
		"start":   {Method: "Start", App: "shop"},
		"restart": {Method: "Start", App: "shop"},
		"stop":    {Method: "Stop", App: "shop"},
		"manual":  {Method: "SetManual", App: "shop", Arg: true},
		"auto":    {Method: "SetManual", App: "shop", Arg: false},
	} {
		b := apps()
		status, _ := post(t, serve(t, b), "/apps/shop", form, nil)
		assert.Equal(t, http.StatusSeeOther, status, form)
		assert.Equal(t, []fake.Call{want}, b.Calls("Start", "Stop", "SetManual"), form)
	}
}

// A name that is not an app of the workspace is 404, and a post to it never reaches the backend.
func TestAppPageUnknownApp(t *testing.T) {
	b := apps()
	srv := serve(t, b)
	status, _ := get(t, srv, "/apps/ledger")
	assert.Equal(t, http.StatusNotFound, status)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/apps/ledger", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", srv.URL)
	status, _ = fetch(t, srv, req)
	assert.Equal(t, http.StatusNotFound, status)
	assert.Empty(t, b.Calls("Start", "Stop", "SetManual"))
}

func TestPersonasPage(t *testing.T) {
	b := &fake.Backend{Data: fake.Data{
		Status: devapi.Status{Apps: []devapi.App{
			{Name: "shop", URL: "http://shop.localhost:8080/"},
			{Name: "billing", CallsOnly: true},
			{Name: "notes", URL: "javascript:alert(1)"},
		}},
		Personas: []devconfig.Persona{
			{Name: "alice", Roles: []string{"hotel-ops", "<b>admin</b>"}, Groups: []string{"lisbon", "porto"}},
			{Name: "no-roles"},
		},
	}}
	status, page := get(t, serve(t, b), "/personas")
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, page, `<a href="/personas">Personas</a>`)
	assert.Contains(t, page, "<tr><td>alice</td><td>hotel-ops, &lt;b&gt;admin&lt;/b&gt;</td><td>lisbon, porto</td></tr>")
	assert.Contains(t, page, "<tr><td>no-roles</td><td></td><td></td></tr>")
	assert.Contains(t, page, `<li><a href="http://shop.localhost:8080/_aicoded/persona">shop</a></li>`)
	assert.Contains(t, page, `<li><a href="about:invalid#aicoded-unsafe-url">notes</a></li>`)
	assert.NotContains(t, page, "billing", "an app with no pages has no persona page")
	assert.NotContains(t, page, "<b>")
}

func TestMCPPage(t *testing.T) {
	for root, args := range map[string]string{
		"/home/me/work":               ` -- '/opt/dev tools/aicoded' mcp /home/me/work`,
		`/home/me/<b>it's "ours"</b>`: ` -- '/opt/dev tools/aicoded' mcp '/home/me/<b>it'\''s "ours"</b>'`,
	} {
		srv := serve(t, &fake.Backend{Data: fake.Data{Status: devapi.Status{Root: root}}})
		status, page := get(t, srv, "/mcp")
		assert.Equal(t, http.StatusOK, status)
		assert.Contains(t, page, `<a href="/mcp">MCP setup</a>`)
		assert.Equal(t, "claude mcp add aicoded"+args, elementText(t, page, "claude"))
		assert.Equal(t, "codex mcp add aicoded"+args, elementText(t, page, "codex"))
		var cursor struct {
			Servers map[string]struct {
				Command string   `json:"command"`
				Args    []string `json:"args"`
			} `json:"mcpServers"`
		}
		require.NoError(t, json.Unmarshal([]byte(elementText(t, page, "cursor")), &cursor))
		assert.Equal(t, executable, cursor.Servers["aicoded"].Command)
		assert.Equal(t, []string{"mcp", root}, cursor.Servers["aicoded"].Args)
		assert.NotContains(t, page, "<b>")
	}
}
