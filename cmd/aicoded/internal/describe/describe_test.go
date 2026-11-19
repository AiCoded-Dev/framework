package describe

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/generate"
	"aicoded.dev/framework/cmd/aicoded/internal/problem"
	"aicoded.dev/framework/cmd/aicoded/internal/surface"
	"aicoded.dev/framework/cmd/aicoded/internal/testhome"
	"aicoded.dev/framework/internal/errs"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// copyWorkspace copies the e2e rpc pair and the dev runner's hello app into a temp folder, with
// their replace directives pointing at this checkout, generates them, and returns the folder, in a
// home of its own with no dev.yaml. It
// skips the test without the go command, which Describe needs to lint the apps.
func copyWorkspace(t *testing.T) string {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	testhome.Set(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	framework, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	require.NoError(t, err)
	for name, src := range map[string]string{
		"billing": "../e2e/testdata/rpc/billing",
		"shop":    "../e2e/testdata/rpc/shop",
		"hello":   "../dev/testdata/hello",
	} {
		require.NoError(t, os.CopyFS(filepath.Join(root, name), os.DirFS(src)))
		gomod := filepath.Join(root, name, "go.mod")
		data, err := os.ReadFile(gomod)
		require.NoError(t, err)
		data = regexp.MustCompile(`(?m)^replace aicoded.dev/framework => .*$`).
			ReplaceAll(data, []byte("replace aicoded.dev/framework => "+framework))
		require.NoError(t, os.WriteFile(gomod, data, 0o600))
	}
	for _, name := range []string{"billing", "shop", "hello"} {
		require.NoError(t, generate.Generate(filepath.Join(root, name)))
	}
	return root
}

func TestDescribe(t *testing.T) {
	root := copyWorkspace(t)
	s, err := Describe(t.Context(), root, "")
	require.NoError(t, err)
	all := []string{"*"}
	everyone := &Audience{Internal: []string{"everyone"}}
	noParams, noCalls, noForms := []generate.ParamInfo{}, []string{}, []generate.FormInfo{}
	assert.Equal(t, Summary{Apps: []App{
		{Name: "billing", Dir: filepath.Join(root, "billing"), Audience: everyone, Serves: []Serve{
			{Name: "Fail", Callers: []string{"shop"}, Require: all},
			{Name: "GetInvoice", Callers: []string{"shop"}, Require: []string{"editor"}},
			{Name: "Missing", Callers: []string{"shop"}, Require: all},
			{Name: "Ping", Callers: []string{"shop"}, Apps: true},
			{Name: "Secret", Callers: []string{"shop"}, Require: []string{"admin"}},
		}, Surface: surface.Surface{Size: 5, EntryPoints: 5}, Local: Local{Note: noWorkspace}},
		{Name: "hello", Dir: filepath.Join(root, "hello"), Audience: everyone, Settings: []string{"greeting"}, Secrets: []string{"token"},
			Local: Local{Note: noWorkspace, Settings: []Value{{Name: "greeting"}}, Secrets: []Value{{Name: "token"}}}},
		{Name: "shop", Dir: filepath.Join(root, "shop"), Audience: everyone, Pages: []Page{
			{Path: "/checks", Require: all},
			{Path: "/invoices/{id}", Require: all, Shared: true},
			{Path: "/status", Require: all},
		}, Calls: map[string][]string{"billing": {"Fail", "GetInvoice", "Missing", "Ping", "Secret"}},
			Surface: surface.Surface{Size: 3, EntryPoints: 3}, Local: Local{Note: noWorkspace},
			Routes: []generate.RouteInfo{
				{Path: "/", Template: "pages/index.html", Layout: true, Params: noParams, Require: all, Calls: noCalls, Forms: noForms},
				{Path: "/checks", Template: "pages/checks/index.html", Params: noParams, Require: all, Calls: noCalls, Forms: noForms},
				{Path: "/invoices/{id}", Template: "pages/invoices/n_id/index.html", Params: []generate.ParamInfo{{Name: "id", Kind: "number"}},
					Require: all, Shared: true, Calls: noCalls, Forms: noForms},
				{Path: "/status", Template: "pages/status/index.html", Params: noParams, Require: all, Calls: noCalls, Forms: noForms},
			}},
	}}, s)

	s, err = Describe(t.Context(), root, "hello")
	require.NoError(t, err)
	require.Len(t, s.Apps, 1)
	data, err := json.Marshal(s)
	require.NoError(t, err)
	assert.JSONEq(t, `{"apps": [{"name": "hello", "dir": `+quote(filepath.Join(root, "hello"))+`, "audience": {"internal": ["everyone"]},
		"settings": ["greeting"], "secrets": ["token"], "surface": {"size": 0, "entry_points": 0, "effects": 0}, "local": `+helloLocal+`}]}`, string(data))
}

// The room-maintenance and people examples, described in text and JSON, match their golden
// files.
func TestDescribeExample(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	testhome.Set(t)
	for _, app := range []string{"room-maintenance", "people"} {
		s, err := Describe(t.Context(), filepath.Join("..", "..", "..", "..", "examples"), app)
		require.NoError(t, err)
		require.Len(t, s.Apps, 1)
		s.Apps[0].Dir = "/apps/" + app
		var text bytes.Buffer
		require.NoError(t, s.WriteText(&text))
		golden(t, text.Bytes(), app+".txt")
		data, err := json.MarshalIndent(s, "", "  ")
		require.NoError(t, err)
		golden(t, append(data, '\n'), app+".json")
	}
}

func golden(t *testing.T, got []byte, name string) {
	t.Helper()
	p := filepath.Join("testdata", name)
	if *update {
		require.NoError(t, os.WriteFile(p, got, 0o600))
	}
	want, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
}

func TestDescribeSurface(t *testing.T) {
	root := copyWorkspace(t)
	notes := `package main

import (
	"context"
	"database/sql"
)

func addNote(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, "INSERT INTO notes (body) VALUES (?)", "hi")
	return err
}
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "hello", "notes.go"), []byte(notes), 0o600))
	s, err := Describe(t.Context(), root, "hello")
	require.NoError(t, err)
	assert.Equal(t, surface.Surface{Size: 1, Effects: 1, Writes: []string{"INSERT notes"}}, s.Apps[0].Surface)
	assert.Empty(t, s.Apps[0].Warnings)

	require.NoError(t, os.WriteFile(filepath.Join(root, "hello", "notes.go"), []byte("package main\n\nvar x = y\n"), 0o600))
	s, err = Describe(t.Context(), root, "hello")
	require.NoError(t, err)
	assert.Equal(t, surface.Surface{}, s.Apps[0].Surface, "the entry points only")
	assert.Equal(t, []string{"the surface counts no SQL writes: lint cannot read the app: lint cannot load package hello: notes.go:3:9: undefined: y"},
		s.Apps[0].Warnings, "positions relative to the app")
}

func TestDescribePrecheck(t *testing.T) {
	root := copyWorkspace(t)
	notes := "package main\n\nimport \"database/sql\"\n\nfunc addNote(db *sql.DB) { _, _ = db.Exec(\"INSERT INTO notes (body) VALUES ('hi')\") }\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "hello", "notes.go"), []byte(notes), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "hello", "vendor"), 0o750))
	s, err := Describe(t.Context(), root, "hello")
	require.NoError(t, err)
	assert.Equal(t, surface.Surface{}, s.Apps[0].Surface, "the entry points only")
	assert.Equal(t, []string{"the surface counts no SQL writes: lint stopped before reading the app: vendor: E-LINT-011: " +
		"the go command builds the app from its vendor folder, which can hold a changed copy of any module"}, s.Apps[0].Warnings)
}

func TestDescribeModules(t *testing.T) {
	root := copyWorkspace(t)
	yaml := filepath.Join(root, "hello", "aicoded.yaml")
	data, err := os.ReadFile(yaml)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(yaml, append(data, "modules: [golang.org/x/text]\n"...), 0o600))

	s, err := Describe(t.Context(), root, "hello")
	require.NoError(t, err)
	data, err = json.Marshal(s)
	require.NoError(t, err)
	assert.JSONEq(t, `{"apps": [{"name": "hello", "dir": `+quote(filepath.Join(root, "hello"))+`, "audience": {"internal": ["everyone"]},
		"settings": ["greeting"], "secrets": ["token"], "modules": ["golang.org/x/text"], "surface": {"size": 0, "entry_points": 0, "effects": 0}, "local": `+helloLocal+`}]}`, string(data))
}

func TestDescribeReach(t *testing.T) {
	root := copyWorkspace(t)
	yaml := filepath.Join(root, "hello", "aicoded.yaml")
	data, err := os.ReadFile(yaml)
	require.NoError(t, err)
	data = bytes.Replace(data, []byte("  internal: [everyone]\n"), []byte("  external: [magic-link]\n"), 1)
	require.NoError(t, os.WriteFile(yaml, append(data, `data:
  - source: connector:crm
    classes: [internal]
egress:
  - hooks.partner.example
  - {host: api.partner.example, secret: token, header: Authorization, prefix: "Bearer "}
schedule:
  - {cron: "0 6 * * *", job: daily-summary}
`...), 0o600))

	s, err := Describe(t.Context(), root, "hello")
	require.NoError(t, err)
	data, err = json.Marshal(s)
	require.NoError(t, err)
	assert.JSONEq(t, `{"apps": [{"name": "hello", "dir": `+quote(filepath.Join(root, "hello"))+`, "audience": {"external": ["magic-link"]},
		"connectors": ["crm"], "egress": ["api.partner.example", "hooks.partner.example"],
		"bindings": [{"host": "api.partner.example", "secret": "token", "header": "Authorization", "prefix": "Bearer "}],
		"schedule": [{"job": "daily-summary", "cron": "0 6 * * *"}], "settings": ["greeting"], "secrets": ["token"],
		"surface": {"size": 3, "entry_points": 1, "effects": 3}, "local": `+helloLocal+`}]}`, string(data))
}

// helloLocal is the local setup of the hello app in a home with no dev.yaml.
var helloLocal = `{"workspace": "", "note": ` + quote(noWorkspace) + `,
	"settings": [{"name": "greeting", "set": false}], "secrets": [{"name": "token", "set": false}]}`

func quote(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

// leaky is a dev.yaml for the workspace root whose values describe must never show.
func leaky(root string) string {
	return "workspaces:\n  " + root + `:
    mysql: root:pw-SHOULD-NOT-LEAK@unix(/var/run/mysqld/mysqld.sock)/
    personas:
      - {name: alice, roles: [editor, admin], groups: [lisbon]}
      - {name: bob}
    apps:
      hello:
        settings: {greeting: set-SHOULD-NOT-LEAK}
        secrets: {token: sec-SHOULD-NOT-LEAK}
`
}

// describeText returns the summary of app under dir, in text and in JSON.
func describeText(t *testing.T, dir, app string) (Summary, string) {
	s, err := Describe(t.Context(), dir, app)
	require.NoError(t, err)
	var text bytes.Buffer
	require.NoError(t, s.WriteText(&text))
	data, err := json.Marshal(s)
	require.NoError(t, err)
	return s, text.String() + string(data)
}

func TestDescribeLocal(t *testing.T) {
	root := copyWorkspace(t)
	yaml := filepath.Join(root, "hello", "aicoded.yaml")
	data, err := os.ReadFile(yaml)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(yaml, append(data, "data:\n  - {source: sqldb, classes: [internal]}\n"...), 0o600))
	testhome.DevYAML(t, os.Getenv("HOME"), leaky(root))

	s, out := describeText(t, root, "hello")
	yes, no := true, false
	assert.Equal(t, Local{Workspace: root, MySQL: &yes,
		Personas: []Persona{{Name: "alice", Roles: []string{"editor", "admin"}, Groups: []string{"lisbon"}}, {Name: "bob"}},
		Settings: []Value{{Name: "greeting", Set: true}}, Secrets: []Value{{Name: "token", Set: true}}}, s.Apps[0].Local)
	assert.Contains(t, out, "  local workspace "+root+"\n  local mysql set\n  local persona alice  roles editor, admin  groups lisbon\n"+
		"  local persona bob\n  local setting greeting set\n  local secret token set\n")
	assert.NotContains(t, out, "SHOULD-NOT-LEAK")

	s, out = describeText(t, filepath.Join(root, "hello"), "hello")
	assert.Equal(t, Local{Note: aboveWorkspace(root), MySQL: &no, Settings: []Value{{Name: "greeting"}}, Secrets: []Value{{Name: "token"}}},
		s.Apps[0].Local, "aicoded dev run in the app's folder uses the defaults, not the workspace above it")
	assert.Contains(t, out, "  local note: dev.yaml names the workspace "+root+", not this folder; aicoded dev uses its values only when it runs there")
	assert.NotContains(t, out, "SHOULD-NOT-LEAK")

	testhome.DevYAML(t, os.Getenv("HOME"), "workspaces:\n  "+root+":\n    port: 9000\n")
	s, out = describeText(t, root, "hello")
	assert.Equal(t, Local{Workspace: root, MySQL: &no, Settings: []Value{{Name: "greeting"}}, Secrets: []Value{{Name: "token"}}}, s.Apps[0].Local)
	assert.Contains(t, out, "  local mysql not set\n  local personas none: aicoded dev signs in as all-roles or no-roles\n"+
		"  local setting greeting not set\n  local secret token not set\n")
}

// A persona's name, roles and groups cannot change the terminal or forge a line of the text.
func TestDescribeLocalText(t *testing.T) {
	root := copyWorkspace(t)
	testhome.DevYAML(t, os.Getenv("HOME"), "workspaces:\n  "+root+`:
    personas:
      - {name: "al\e[31mice\n  local secret token set", roles: ["a\u202eb"], groups: ["g\rh"]}
`)
	_, out := describeText(t, root, "hello")
	text, _, _ := strings.Cut(out, "{")
	assert.Contains(t, text, "  local persona al?[31mice?  local secret token set  roles a?b  groups g?h\n")
	assert.NotContains(t, text, "\x1b")
	assert.NotContains(t, text, "\n  local secret token set\n")
	assert.Contains(t, text, "\n  local secret token not set\n")
}

// A dev.yaml that aicoded dev refuses is a problem of the local setup, which quotes none of its
// values.
func TestDescribeLocalProblem(t *testing.T) {
	root := copyWorkspace(t)
	home := os.Getenv("HOME")
	path := filepath.Join(home, ".config", "aicoded", "dev.yaml")
	for _, tc := range []struct {
		content, code string
		mode          os.FileMode
	}{
		{leaky(root), "E-DEV-001", 0o644},
		{"workspaces:\n  " + root + ":\n    apps:\n      hello:\n        secrets: {token: [sec-SHOULD-NOT-LEAK}\n", "E-DEV-002", 0o600},
		{"workspaces:\n  " + root + ":\n    port: pw-SHOULD-NOT-LEAK\n", "E-DEV-002", 0o600},
	} {
		testhome.DevYAML(t, home, tc.content)
		require.NoError(t, os.Chmod(path, tc.mode))
		s, out := describeText(t, root, "hello")
		require.NotNil(t, s.Apps[0].Local.Problem)
		assert.Equal(t, tc.code, s.Apps[0].Local.Problem.Code)
		assert.Regexp(t, `\n  local problem: (\S+: )?`+tc.code+`: `, out)
		assert.NotContains(t, out, "SHOULD-NOT-LEAK")
	}
}

func TestDescribeFindsStaleFiles(t *testing.T) {
	root := copyWorkspace(t)
	route := filepath.Join(root, "shop", "pages", "route_gen.go")
	data, err := os.ReadFile(route)
	require.NoError(t, err)
	edited := append(data, "// edited\n"...)
	require.NoError(t, os.WriteFile(route, edited, 0o600))

	s, err := Describe(t.Context(), root, "shop")
	require.NoError(t, err)
	require.Len(t, s.Apps, 1)
	require.Len(t, s.Apps[0].Findings, 1)
	f := s.Apps[0].Findings[0]
	assert.Equal(t, problem.Problem{App: "shop", Code: "E-CHK-001", Pos: "pages/route_gen.go:1", Message: "pages/route_gen.go is out of date"},
		problem.Problem{App: f.App, Code: f.Code, Pos: f.Pos, Message: f.Message})
	after, err := os.ReadFile(route)
	require.NoError(t, err)
	assert.Equal(t, edited, after, "describe never writes")
}

func TestDescribeUnknownApp(t *testing.T) {
	_, err := Describe(t.Context(), copyWorkspace(t), "ledger")
	assert.Equal(t, "E-DEV-014", errs.Code(err))
	_, err = Describe(t.Context(), t.TempDir(), "")
	assert.Equal(t, "E-MAN-006", errs.Code(err))
}

func TestDescribeText(t *testing.T) {
	s := Summary{Apps: []App{
		{Name: "billing", Dir: "/w/billing", Serves: []Serve{
			{Name: "GetInvoice", Callers: []string{"shop", "desk"}, Require: []string{"editor", "admin|finance"}},
			{Name: "Ping", Callers: []string{"shop"}, Apps: true},
		}, Surface: surface.Surface{Size: 2, EntryPoints: 2}},
		{
			Name: "shop", Dir: "/w/shop",
			Pages: []Page{
				{Path: "/", Require: []string{"*"}},
				{Path: "/orders/{id}", Require: []string{"editor", "admin"}, Guard: true, Calls: []string{"Cancel", "Pay"}},
				{Path: "/tags/{tag}", Require: []string{"editor"}, Shared: true},
			},
			Calls:    map[string][]string{"ledger": {"Post"}, "billing": {"GetInvoice", "Ping"}},
			SQLDB:    true,
			Stores:   []string{"docs", "photos"},
			Email:    &Email{From: "shop@acme.example", ToDomains: []string{"acme.example", "partner.example"}},
			Settings: []string{"report_day"},
			Secrets:  []string{"partner_api_key"},
			Modules:  []string{"golang.org/x/text"},
			Surface:  surface.Surface{Size: 4, EntryPoints: 4, Effects: 3, Writes: []string{"INSERT orders"}},
			Warnings: []string{"pages/x.ts:3: duplicate key"},
			Findings: problem.From("shop", errs.At("pages/route_gen.go:1", "E-CHK-001", "pages/route_gen.go is out of date", "run aicoded generate")),
		},
		{Name: "tiny", Dir: "/w/tiny", Email: &Email{From: "tiny@acme.example"}},
		{
			Name: "ops", Dir: "/w/ops", Class: "internal-tool", Owner: "group:ops-leads",
			Audience:   &Audience{Internal: []string{"group:ops", "everyone"}, External: []string{"idp:partners", "magic-link"}},
			Pages:      []Page{{Path: "/", Require: []string{"*"}}},
			Stores:     []string{"photos"},
			Connectors: []string{"crm", "m365-sharepoint"},
			Email:      &Email{From: "ops@acme.example", ToDomains: []string{"acme.example"}},
			Egress:     []string{"api.partner.example", "hooks.partner.example", "maps.partner.example"},
			Bindings: []Binding{
				{Host: "api.partner.example", Secret: "partner_api_key", Header: "Authorization", Prefix: "Bearer "},
				{Host: "maps.partner.example", Secret: "maps_key", Header: "X-Api-Key"},
			},
			Schedule: []Job{{Name: "daily-summary", Cron: "0 6 * * *"}, {Name: "sync", Cron: "*/15 * * * *"}},
			Settings: []string{"report_day"},
			Modules:  []string{"golang.org/x/text"},
			Size:     "M", Resources: "medium", TTL: "90d",
			Surface: surface.Surface{Size: 5, EntryPoints: 3, Effects: 5},
		},
	}}
	var out bytes.Buffer
	require.NoError(t, s.WriteText(&out))
	assert.Equal(t, `app billing (/w/billing)
  serves GetInvoice  callers shop, desk  require editor, admin|finance
  serves Ping  callers shop  apps
  surface 2 (2 entry points, 0 effects)
app shop (/w/shop)
  page /  require *
  page /orders/{id}  require editor, admin  guard  calls Cancel, Pay
  page /tags/{tag}  require editor  shared
  calls billing: GetInvoice, Ping
  calls ledger: Post
  database
  files docs, photos
  email from shop@acme.example to acme.example, partner.example
  settings report_day
  secrets partner_api_key
  modules golang.org/x/text
  writes INSERT orders
  surface 4 (4 entry points, 3 effects)
  warning: pages/x.ts:3: duplicate key
  finding: shop: pages/route_gen.go:1: E-CHK-001: pages/route_gen.go is out of date
    fix: run aicoded generate
    docs: https://aicoded.dev/docs/errors/E-CHK-001
app tiny (/w/tiny)
  email from tiny@acme.example
  surface 0 (0 entry points, 0 effects)
app ops (/w/ops)
  class internal-tool
  owner group:ops-leads
  audience group:ops, everyone
  external audience idp:partners, magic-link
  page /  require *
  files photos
  connectors crm, m365-sharepoint
  email from ops@acme.example to acme.example
  egress api.partner.example, hooks.partner.example, maps.partner.example
  binding api.partner.example  secret partner_api_key  header Authorization  prefix "Bearer "
  binding maps.partner.example  secret maps_key  header X-Api-Key
  schedule daily-summary  cron 0 6 * * *
  schedule sync  cron */15 * * * *
  settings report_day
  modules golang.org/x/text
  size M
  resources medium
  ttl 90d
  surface 5 (3 entry points, 5 effects)
`, out.String())
}
