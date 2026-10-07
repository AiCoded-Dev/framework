package manifest_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/manifest"
)

func write(t *testing.T, src string) string {
	path := filepath.Join(t.TempDir(), manifest.FileName)
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))
	return path
}

func load(t *testing.T, src string) manifest.Manifest {
	m, err := manifest.Load(write(t, src))
	require.NoError(t, err)
	return m
}

func TestLoad(t *testing.T) {
	m, err := manifest.Load(write(t, "app: rooms\nowner: group:ops\nsettings: [greeting]\nsecrets: [api_key]\nmodules: [golang.org/x/text]\n"))
	require.NoError(t, err)
	assert.Equal(t, manifest.Manifest{App: "rooms", Settings: []string{"greeting"}, Secrets: []string{"api_key"},
		Modules: []string{"golang.org/x/text"}}, m)
}

func TestLoadErrors(t *testing.T) {
	for src, want := range map[string]string{
		"":                                     ":1: E-MAN-004",
		"app: Rooms\n":                         ":1: E-MAN-004",
		"app: rooms\nsettings: [Bad-Name]\n":   ":2: E-MAN-005",
		"app: rooms\nsecrets:\n  - a\n  - a\n": ":4: E-MAN-005",
		"app: [\n":                             ":1: E-MAN-003",
		"app: rooms\nsettings: [\n":            ":2: E-MAN-003",
		"app: rooms\nsettings: greeting\n":     ":2: E-MAN-003",
		"app: rooms\n# c\n---\n":               ":3: E-MAN-003",
		"app: rooms\n---\n[\n":                 ":3: E-MAN-003",
		"app: rooms\nmodules: [os]\n":          ":2: E-MAN-013",
		"app: rooms\nmodules: [golang.org/x/text@v0.3.0]\n":                        ":2: E-MAN-013",
		"app: rooms\nmodules:\n  - golang.org/x/text\n  - aicoded.dev/framework\n": ":4: E-MAN-013",
		"app: rooms\nmodules:\n  - golang.org/x/text\n  - golang.org/x/text\n":     ":4: E-MAN-013",
	} {
		_, err := manifest.Load(write(t, src))
		require.ErrorContains(t, err, want, "source %q", src)
	}
	_, err := manifest.Load(filepath.Join(t.TempDir(), manifest.FileName))
	assert.Equal(t, "E-MAN-006", errs.Code(err))
}

func TestParse(t *testing.T) {
	m, err := manifest.Parse("apps/rooms/aicoded.yaml", []byte("app: rooms\n"))
	require.NoError(t, err)
	assert.Equal(t, "rooms", m.App)

	_, err = manifest.Parse("apps/rooms/aicoded.yaml", []byte("app: rooms\nsettings: [Bad]\n"))
	require.ErrorContains(t, err, "apps/rooms/aicoded.yaml:2: E-MAN-005")
}

func TestModuleBesideFramework(t *testing.T) {
	assert.Equal(t, []string{"aicoded.dev/frameworkx"}, load(t, "app: rooms\nmodules: [aicoded.dev/frameworkx]\n").Modules,
		"a module whose path only starts with the framework's is a third-party module")
}

func TestLoadOneDocument(t *testing.T) {
	assert.Equal(t, "rooms", load(t, "---\napp: rooms\n").App)

	path := write(t, "app: rooms\n---\napp: other\n")
	_, err := manifest.Load(path)
	var e *errs.Error
	require.ErrorAs(t, err, &e)
	assert.Equal(t, path+":2 E-MAN-003", e.Pos+" "+e.Code)
	assert.Equal(t, "aicoded.yaml has more than one YAML document", e.Msg)
}

func TestDataAndEmail(t *testing.T) {
	m := load(t, `app: blocks
data:
  - source: sqldb
    classes: [internal]
  - source: filestore:invoices
    classes: [internal, finance]
  - source: connector:m365-sharepoint
    classes: [internal]
email:
  from: ops@acme.example
  to_domains: [acme.example, partner.example]
`)
	assert.True(t, m.SQLDB())
	assert.Equal(t, []string{"invoices"}, m.Stores())
	assert.Equal(t, &manifest.Email{From: "ops@acme.example", ToDomains: []string{"acme.example", "partner.example"}}, m.Email)

	m = load(t, "app: plain\n")
	assert.False(t, m.SQLDB())
	assert.Empty(t, m.Stores())
	assert.Nil(t, m.Email)
}

func TestDataAndEmailErrors(t *testing.T) {
	type at struct {
		line int
		code string
	}
	for yml, want := range map[string]at{
		"app: a1\ndata:\n  - source: postgres\n    classes: [internal]\n":                                   {3, "E-MAN-007"},
		"app: a1\ndata:\n  - source: filestore:Bad_Name\n    classes: [internal]\n":                         {3, "E-MAN-007"},
		"app: a1\ndata:\n  - source: sqldb\n    classes: [internal]\n  - source: sqldb\n    classes: [x]\n": {5, "E-MAN-007"},
		"app: a1\ndata:\n  - source: sqldb\n":                                                               {3, "E-MAN-008"},
		"app: a1\ndata:\n  - source: sqldb\n    classes: [Internal]\n":                                      {3, "E-MAN-008"},
		"app: a1\nemail:\n  from: Ops <ops@acme.example>\n  to_domains: [acme.example]\n":                   {2, "E-MAN-009"},
		"app: a1\nemail:\n  from: ops@acme.example\n":                                                       {2, "E-MAN-009"},
		"app: a1\nemail:\n  from: ops@acme.example\n  to_domains: [\"*.acme.example\"]\n":                   {2, "E-MAN-009"},
		"app: a1\nemail:\n  from: ops@bücher.example\n  to_domains: [acme.example]\n":                       {2, "E-MAN-009"},
		"app: a1\nemail:\n  from: o..ps@acme.example\n  to_domains: [acme.example]\n":                       {2, "E-MAN-009"},
		"app: a1\nemail:\n  from: Ops@acme.example\n  to_domains: [acme.example]\n":                         {2, "E-MAN-009"},
		"app: a1\nemail:\n  from: ops@acme.example\n  to_domains: [Acme.example]\n":                         {2, "E-MAN-009"},
	} {
		path := write(t, yml)
		_, err := manifest.Load(path)
		var e *errs.Error
		require.ErrorAs(t, err, &e, yml)
		assert.Equal(t, fmt.Sprintf("%s:%d %s", path, want.line, want.code), e.Pos+" "+e.Code, yml)
	}
}

func TestRoles(t *testing.T) {
	m := load(t, `app: notes
access:
  "/":
    require: ["*"]
  "/notes/{id}":
    require: ["editor", "admin|owner"]
    guard: true
services:
  serves:
    "Get":
      callers: ["shop"]
      require: ["auditor|editor"]
    "Ping":
      callers: ["shop"]
      apps: true
`)
	assert.Equal(t, []string{"admin", "auditor", "editor", "owner"}, m.Roles())
}

func TestServices(t *testing.T) {
	m := load(t, `app: shop
services:
  calls:
    "billing": ["GetInvoice", "Ping"]
  serves:
    "Order":
      callers: ["reports"]
      require: ["*"]
      apps: true
`)
	assert.Equal(t, manifest.Services{
		Calls:  map[string][]string{"billing": {"GetInvoice", "Ping"}},
		Serves: map[string]manifest.Serve{"Order": {Callers: []string{"reports"}, Require: []string{"*"}, Apps: true}},
	}, m.Services)
	assert.Equal(t, manifest.Services{}, load(t, "app: shop\n").Services)
}

func TestValidApp(t *testing.T) {
	for _, name := range []string{"ab", "billing", "hotel-ops-2"} {
		assert.True(t, manifest.ValidApp(name), name)
	}
	for _, name := range []string{"", "b", "Billing", "2shop", "shop-", "bill_ing", "../shop", "shop\"", "a123456789012345678901234567890123456789012345678901234567890123"} {
		assert.False(t, manifest.ValidApp(name), name)
	}
}
