package describe

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/generate"
	"aicoded.dev/framework/cmd/aicoded/internal/problem"
	"aicoded.dev/framework/cmd/aicoded/internal/surface"
	"aicoded.dev/framework/internal/errs"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// copyWorkspace copies the e2e rpc pair and the dev runner's hello app into a temp folder, with
// their replace directives pointing at this checkout, generates them, and returns the folder. It
// skips the test without the go command, which Describe needs to lint the apps.
func copyWorkspace(t *testing.T) string {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
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
	assert.Equal(t, Summary{Apps: []App{
		{Name: "billing", Dir: filepath.Join(root, "billing"), Serves: []Serve{
			{Name: "Fail", Callers: []string{"shop"}, Require: all},
			{Name: "GetInvoice", Callers: []string{"shop"}, Require: []string{"editor"}},
			{Name: "Missing", Callers: []string{"shop"}, Require: all},
			{Name: "Ping", Callers: []string{"shop"}, Apps: true},
			{Name: "Secret", Callers: []string{"shop"}, Require: []string{"admin"}},
		}, Surface: surface.Surface{Size: 5, EntryPoints: 5}},
		{Name: "hello", Dir: filepath.Join(root, "hello"), Settings: []string{"greeting"}, Secrets: []string{"token"}},
		{Name: "shop", Dir: filepath.Join(root, "shop"), Pages: []Page{
			{Path: "/checks", Require: all},
			{Path: "/invoices/{id}", Require: all},
			{Path: "/status", Require: all},
		}, Calls: map[string][]string{"billing": {"Fail", "GetInvoice", "Missing", "Ping", "Secret"}},
			Surface: surface.Surface{Size: 3, EntryPoints: 3}},
	}}, s)

	s, err = Describe(t.Context(), root, "hello")
	require.NoError(t, err)
	require.Len(t, s.Apps, 1)
	data, err := json.Marshal(s)
	require.NoError(t, err)
	assert.JSONEq(t, `{"apps": [{"name": "hello", "dir": `+quote(filepath.Join(root, "hello"))+`, "settings": ["greeting"], "secrets": ["token"],
		"surface": {"size": 0, "entry_points": 0, "effects": 0}}]}`, string(data))
}

// The room-maintenance example, described in text and JSON, matches its golden files.
func TestDescribeExample(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	s, err := Describe(t.Context(), filepath.Join("..", "..", "..", "..", "examples"), "room-maintenance")
	require.NoError(t, err)
	require.Len(t, s.Apps, 1)
	s.Apps[0].Dir = "/apps/room-maintenance"
	var text bytes.Buffer
	require.NoError(t, s.WriteText(&text))
	golden(t, text.Bytes(), "room-maintenance.txt")
	data, err := json.MarshalIndent(s, "", "  ")
	require.NoError(t, err)
	golden(t, append(data, '\n'), "room-maintenance.json")
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
	assert.JSONEq(t, `{"apps": [{"name": "hello", "dir": `+quote(filepath.Join(root, "hello"))+`, "settings": ["greeting"], "secrets": ["token"],
		"modules": ["golang.org/x/text"], "surface": {"size": 0, "entry_points": 0, "effects": 0}}]}`, string(data))
}

func TestDescribeReach(t *testing.T) {
	root := copyWorkspace(t)
	yaml := filepath.Join(root, "hello", "aicoded.yaml")
	data, err := os.ReadFile(yaml)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(yaml, append(data, `audience:
  external: [magic-link]
data:
  - source: connector:crm
    classes: [internal]
egress: [api.partner.example, hooks.partner.example]
schedule:
  - {cron: "0 6 * * *", job: daily-summary}
`...), 0o600))

	s, err := Describe(t.Context(), root, "hello")
	require.NoError(t, err)
	data, err = json.Marshal(s)
	require.NoError(t, err)
	assert.JSONEq(t, `{"apps": [{"name": "hello", "dir": `+quote(filepath.Join(root, "hello"))+`, "audience": {"external": ["magic-link"]},
		"connectors": ["crm"], "egress": ["api.partner.example", "hooks.partner.example"],
		"schedule": [{"job": "daily-summary", "cron": "0 6 * * *"}], "settings": ["greeting"], "secrets": ["token"],
		"surface": {"size": 3, "entry_points": 1, "effects": 3}}]}`, string(data))
}

func quote(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
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
			Egress:     []string{"api.partner.example", "hooks.partner.example"},
			Schedule:   []Job{{Name: "daily-summary", Cron: "0 6 * * *"}, {Name: "sync", Cron: "*/15 * * * *"}},
			Settings:   []string{"report_day"},
			Modules:    []string{"golang.org/x/text"},
			Size:       "M", Resources: "medium", TTL: "90d",
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
  egress api.partner.example, hooks.partner.example
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
