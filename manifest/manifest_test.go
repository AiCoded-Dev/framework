package manifest_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	assert.Equal(t, manifest.Manifest{App: "rooms", Owner: "group:ops", Settings: []string{"greeting"}, Secrets: []string{"api_key"},
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

// problems returns the position and code of every problem Parse reports in src, in order.
func problems(t *testing.T, src string) []string {
	_, err := manifest.Parse(manifest.FileName, []byte(src))
	require.Error(t, err, src)
	var out []string
	for _, err := range err.(interface{ Unwrap() []error }).Unwrap() {
		var e *errs.Error
		require.ErrorAs(t, err, &e)
		out = append(out, e.Pos+" "+e.Code)
	}
	return out
}

func TestFullList(t *testing.T) {
	m, err := manifest.Load(filepath.Join("testdata", "full.yaml"))
	require.NoError(t, err)
	assert.Equal(t, manifest.Manifest{
		App:      "room-maintenance",
		Class:    "internal-ops",
		Owner:    "group:hotel-ops-leads",
		Audience: manifest.Audience{Internal: []string{"group:hotel-ops-lisbon"}, External: []string{}},
		Data: []manifest.Data{
			{Source: "connector:m365-sharepoint", Classes: []string{"internal"}},
			{Source: "sqldb", Classes: []string{"internal"}},
			{Source: "filestore:invoices", Classes: []string{"internal"}},
		},
		Access: map[string]manifest.Access{
			"/rooms":           {Require: []string{"hotel-ops"}},
			"/rooms/{id}/edit": {Require: []string{"hotel-ops"}, Guard: true},
		},
		Services: manifest.Services{
			Calls:  map[string][]string{"inventory": {"ReserveParts"}},
			Serves: map[string]manifest.Serve{"RoomStatus": {Callers: []string{"front-desk"}, Require: []string{"hotel-ops"}}},
		},
		Egress:    []string{},
		Email:     &manifest.Email{From: "ops@acme.example", ToDomains: []string{"acme.example"}},
		Schedule:  []manifest.Job{{Cron: "0 6 * * *", Name: "daily-summary"}},
		Settings:  []string{"report_day"},
		Secrets:   []string{"partner_api_key"},
		Modules:   []string{},
		Size:      "S",
		Resources: "small",
		TTL:       "180d",
	}, m)
}

func TestFields(t *testing.T) {
	m := load(t, `app: a1
owner: group:ops.lisbon
audience:
  internal: [everyone, group:hr_leads]
  external: [idp:partners, magic-link]
egress: [api.partner.example, hooks.slack.com]
ttl: 1d
`)
	assert.Equal(t, "group:ops.lisbon", m.Owner)
	assert.Equal(t, manifest.Audience{Internal: []string{"everyone", "group:hr_leads"}, External: []string{"idp:partners", "magic-link"}}, m.Audience)
	assert.Equal(t, []string{"api.partner.example", "hooks.slack.com"}, m.Egress)
	assert.Equal(t, "1d", m.TTL)
	assert.Equal(t, "group:hr_leads", load(t, "app: a1\nowner: group:hr_leads\n").Owner)
	assert.Equal(t, "3650d", load(t, "app: a1\nttl: 3650d\n").TTL)

	empty := "app: a1\nclass:\nowner: ''\naudience:\negress: []\nschedule:\nsize:\nresources: ''\nttl: ~\n"
	assert.Equal(t, manifest.Manifest{App: "a1", Egress: []string{}}, load(t, empty), "an empty value is not stated")
}

func TestFieldErrors(t *testing.T) {
	for src, want := range map[string]string{
		"class: Internal":               "2 E-MAN-015",
		"class: internal_tool":          "2 E-MAN-015",
		"class: -ops":                   "2 E-MAN-015",
		"owner: hr-leads":               "2 E-MAN-016",
		"owner: 'group:'":               "2 E-MAN-016",
		"owner: group:HR":               "2 E-MAN-016",
		"owner: group:hr leads":         "2 E-MAN-016",
		"owner:\n  user:a@acme.example": "3 E-MAN-016",

		"audience:\n  internal:\n    - idp:partners":                 "4 E-MAN-017",
		"audience:\n  internal:\n    - magic-link":                   "4 E-MAN-017",
		"audience:\n  internal:\n    - staff":                        "4 E-MAN-017",
		"audience:\n  internal:\n    - everyone\n    - everyone":     "5 E-MAN-017",
		"audience:\n  external:\n    - everyone":                     "4 E-MAN-017",
		"audience:\n  external:\n    - group:x":                      "4 E-MAN-017",
		"audience:\n  external:\n    - idp:Partners":                 "4 E-MAN-017",
		"audience:\n  external:\n    - magic-link\n    - magic-link": "5 E-MAN-017",

		"egress:\n  - https://api.partner.example":              "3 E-MAN-018",
		"egress:\n  - api.partner.example:443":                  "3 E-MAN-018",
		"egress:\n  - api.partner.example/v1":                   "3 E-MAN-018",
		"egress:\n  - '*.partner.example'":                      "3 E-MAN-018",
		"egress:\n  - 10.0.0.1":                                 "3 E-MAN-018",
		"egress:\n  - API.partner.example":                      "3 E-MAN-018",
		"egress:\n  - localhost":                                "3 E-MAN-018",
		"egress:\n  - printer.local":                            "3 E-MAN-018",
		"egress:\n  - metadata.google.internal":                 "3 E-MAN-018",
		"egress:\n  - 1.0.0.10.in-addr.arpa":                    "3 E-MAN-018",
		"egress:\n  - partner.example.":                         "3 E-MAN-018",
		"egress:\n  - a.example\n  - a.example":                 "4 E-MAN-018",
		"egress:\n  - " + strings.Repeat("a.", 126) + "example": "3 E-MAN-018",

		"size: s":          "2 E-MAN-020",
		"size: XL":         "2 E-MAN-020",
		"resources: Small": "2 E-MAN-021",
		"resources: huge":  "2 E-MAN-021",
		"ttl: 0d":          "2 E-MAN-022",
		"ttl: 01d":         "2 E-MAN-022",
		"ttl: 180":         "2 E-MAN-022",
		"ttl: 6m":          "2 E-MAN-022",
		"ttl: 1y":          "2 E-MAN-022",
		"ttl: -5d":         "2 E-MAN-022",
		"ttl:\n  3651d":    "3 E-MAN-022",

		"schedule:\n  - job: daily":                                                   "3 E-MAN-019",
		"schedule:\n  - cron: '0 6 * * *'":                                            "3 E-MAN-019",
		"schedule:\n  - cron: '0 6 * * *'\n    job: Daily":                            "4 E-MAN-019",
		"schedule:\n  - job: daily\n    cron: '0 25 * * *'":                           "4 E-MAN-019",
		"schedule:\n  - {cron: '0 6 * * *', job: a}\n  - {cron: '0 7 * * *', job: a}": "4 E-MAN-019",
	} {
		assert.Equal(t, []string{"aicoded.yaml:" + want}, problems(t, "app: a1\n"+src+"\n"), src)
	}
}

func TestFieldMessages(t *testing.T) {
	for src, want := range map[string]string{
		"egress: ['https://api.partner.example']":              "egress entry \"https://api.partner.example\" is not a host name",
		"egress: [localhost]":                                  "egress entry \"localhost\" is a local name, which an app may not reach",
		"egress: [metadata.google.internal]":                   "egress entry \"metadata.google.internal\" is a local name, which an app may not reach",
		"egress: [a.example, a.example]":                       "egress entry \"a.example\" is listed twice",
		"schedule: [{cron: '0 25 * * *', job: daily-summary}]": "schedule job \"daily-summary\": cron \"0 25 * * *\" has hour 25, outside 0-23",
		"schedule: [{cron: '0 6 * * *'}]":                      "schedule entry with cron \"0 6 * * *\" has no job",
		"schedule: [{job: daily}]":                             "schedule job \"daily\" has no cron",
		"ttl: 6m":                                              "ttl \"6m\" is not a number of days",
		"ttl: 3651d":                                           "ttl \"3651d\" is more than 3650 days",
		"class: Ops":                                           "class \"Ops\" is not a valid app type name",
		"owner: hr":                                            "owner \"hr\" is not group:<name>",
		"audience: {internal: [staff]}":                        "audience.internal entry \"staff\" is not group:<name> or everyone",
		"audience: {external: [everyone]}":                     "audience.external entry \"everyone\" is not idp:<name> or magic-link",
		"size: XL":                                             "size \"XL\" is not S, M or L",
		"resources: huge":                                      "resources \"huge\" is not small, medium or large",
	} {
		_, err := manifest.Parse(manifest.FileName, []byte("app: a1\n"+src+"\n"))
		var e *errs.Error
		require.ErrorAs(t, err, &e, src)
		assert.Equal(t, want, e.Msg, src)
	}
	_, err := manifest.Parse(manifest.FileName, []byte("app: a1\nsize: XL\n"))
	assert.ErrorContains(t, err, "fix: use S, M or L")
}

func TestUnknownKeys(t *testing.T) {
	const generated = "remove it: aicoded generate writes this section from the code, so never edit it"
	for src, want := range map[string]struct{ pos, msg, fix string }{
		"owners: x": {":2", `unknown key "owners" in the top level`,
			"remove it, or correct its name: the keys there are app, class, owner, audience, data, access, services, egress, email, schedule, settings, secrets, modules, size, resources, ttl"},
		"audience:\n  inside: [everyone]":                                                            {":3", `unknown key "inside" in audience`, "remove it, or correct its name: the keys there are internal, external"},
		"data:\n  - source: sqldb\n    classes: [internal]\n    class: x":                            {":5", `unknown key "class" in data`, "remove it, or correct its name: the keys there are source, classes"},
		"access:\n  \"/\":\n    require: [\"*\"]\n    roles: [x]":                                    {":5", `unknown key "roles" in access "/"`, generated},
		"services:\n  call: {}":                                                                      {":3", `unknown key "call" in services`, generated},
		"services:\n  serves:\n    Get:\n      callers: [shop]\n      apps: true\n      caller: [x]": {":7", `unknown key "caller" in services.serves "Get"`, generated},
		"email:\n  from: a@acme.example\n  to_domains: [acme.example]\n  cc: x":                      {":5", `unknown key "cc" in email`, "remove it, or correct its name: the keys there are from, to_domains"},
		"schedule:\n  - cron: '0 6 * * *'\n    job: daily\n    at: x":                                {":5", `unknown key "at" in schedule`, "remove it, or correct its name: the keys there are cron, job"},
	} {
		_, err := manifest.Parse(manifest.FileName, []byte("app: a1\n"+src+"\n"))
		var e *errs.Error
		require.ErrorAs(t, err, &e, src)
		assert.Equal(t, manifest.FileName+want.pos+" E-MAN-014", e.Pos+" "+e.Code, src)
		assert.Equal(t, want.msg, e.Msg, src)
		assert.Equal(t, want.fix, e.Fix, src)
	}
}

func TestEveryProblemAtOnce(t *testing.T) {
	src := "app: a1\ncolour: red\nsettings: [x]\negress:\n  - localhost\nsecrets: [k]\nsize: S\nresources: small\nttl: 6m\n"
	assert.Equal(t, []string{"aicoded.yaml:2 E-MAN-014", "aicoded.yaml:5 E-MAN-018", "aicoded.yaml:9 E-MAN-022"}, problems(t, src))
	_, err := manifest.Parse(manifest.FileName, []byte(src))
	assert.Equal(t, "E-MAN-014", errs.Code(err))

	assert.Equal(t, []string{"aicoded.yaml:1 E-MAN-014", "aicoded.yaml:1 E-MAN-004", "aicoded.yaml:1 E-MAN-014", "aicoded.yaml:1 E-MAN-020", "aicoded.yaml:1 E-MAN-022"},
		problems(t, "{ttl: 6m, audience: {others: []}, size: XL, colour: red, app: A}\n"), "on one line, in the order of the fields")
	assert.Equal(t, []string{"aicoded.yaml:2 E-MAN-014", "aicoded.yaml:3 E-MAN-003"},
		problems(t, "app: a1\ncolour: red\nsettings: x\nsize: XL\n"), "a shape that does not decode stops the checks")
	assert.Equal(t, []string{"aicoded.yaml:2 E-MAN-005", "aicoded.yaml:3 E-MAN-005"},
		problems(t, "app: a1\nsettings: [Bad]\nsecrets: [Bad]\n"))
	assert.Equal(t, []string{"aicoded.yaml:2 E-MAN-003"}, problems(t, "app: a1\nsettings: [\n"))
}

func TestNoAnchors(t *testing.T) {
	for src, want := range map[string][]string{
		"app: a1\nsettings: &a [x]\n":              {":2"},
		"app: a1\nsettings: &a [x]\nsecrets: *a\n": {":2", ":3"},
		"app: a1\ndata:\n  - &d {source: sqldb, classes: [internal]}\n  - <<: *d\n    source: filestore:docs\n": {":3", ":4"},
		"app: a1\nemail:\n  <<: {from: a@acme.example, to_domains: [acme.example]}\n":                           {":3"},
	} {
		_, err := manifest.Parse(manifest.FileName, []byte(src))
		var got []string
		for _, err := range err.(interface{ Unwrap() []error }).Unwrap() {
			var e *errs.Error
			require.ErrorAs(t, err, &e, src)
			assert.Equal(t, "E-MAN-003", e.Code, src)
			assert.Equal(t, "aicoded.yaml uses an anchor, alias or merge key", e.Msg, src)
			assert.Equal(t, "write each value out in full: aicoded.yaml may not use &name, *name or <<", e.Fix, src)
			got = append(got, strings.TrimPrefix(e.Pos, manifest.FileName))
		}
		assert.Equal(t, want, got, src)
	}
}

func TestAliasBomb(t *testing.T) {
	var b strings.Builder
	b.WriteString("app: a1\nsettings: &a0 [x, x, x, x, x, x, x, x, x, x]\n")
	for i := 1; i < 10; i++ {
		fmt.Fprintf(&b, "k%d: &a%d [", i, i)
		for j := range 10 {
			if j > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "*a%d", i-1)
		}
		b.WriteString("]\n")
	}
	require.Less(t, b.Len(), 64<<10)
	start := time.Now()
	_, err := manifest.Parse(manifest.FileName, []byte(b.String()))
	assert.Less(t, time.Since(start), time.Second)
	assert.Equal(t, "E-MAN-003", errs.Code(err))
}
