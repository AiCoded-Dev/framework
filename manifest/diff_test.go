package manifest_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/manifest"
)

func full(t *testing.T) manifest.Manifest {
	m, err := manifest.Load(filepath.Join("testdata", "full.yaml"))
	require.NoError(t, err)
	return m
}

func TestDiffFullList(t *testing.T) {
	assert.Nil(t, manifest.Diff(full(t), full(t)))
	assert.Equal(t, []manifest.Change{
		{Field: "app", After: "room-maintenance"},
		{Field: "class", After: "internal-ops"},
		{Field: "owner", After: "group:hotel-ops-leads"},
		{Field: "audience.internal", After: "group:hotel-ops-lisbon"},
		{Field: "data", Key: "connector:m365-sharepoint", After: "internal"},
		{Field: "data", Key: "filestore:invoices", After: "internal"},
		{Field: "data", Key: "sqldb", After: "internal"},
		{Field: "access", Key: "/rooms", After: "require hotel-ops"},
		{Field: "access", Key: "/rooms/{id}/edit", After: "require hotel-ops; guard"},
		{Field: "services.calls", Key: "inventory", After: "ReserveParts"},
		{Field: "services.serves", Key: "RoomStatus", After: "callers front-desk; require hotel-ops"},
		{Field: "email.from", After: "ops@acme.example"},
		{Field: "email.to_domains", After: "acme.example"},
		{Field: "schedule", Key: "daily-summary", After: "0 6 * * *"},
		{Field: "settings", After: "report_day"},
		{Field: "secrets", After: "partner_api_key"},
		{Field: "size", After: "S"},
		{Field: "resources", After: "small"},
		{Field: "ttl", After: "180d"},
	}, manifest.Diff(manifest.Manifest{}, full(t)))
}

func TestDiff(t *testing.T) {
	parse := func(src string) manifest.Manifest {
		m, err := manifest.Parse(manifest.FileName, []byte("app: a1\n"+src+"\n"))
		require.NoError(t, err, src)
		return m
	}
	for name, c := range map[string]struct {
		before, after string
		want          []manifest.Change
	}{
		"a new order":         {"settings: [a, b]", "settings: [b, a]", nil},
		"a repeated class":    {"data: [{source: sqldb, classes: [internal, internal]}]", "data: [{source: sqldb, classes: [internal]}]", nil},
		"empty is not stated": {"", "audience: {internal: []}\negress: []\naccess: {}\nservices: {calls: {inventory: []}}", nil},
		"a host":              {"", "egress: [api.partner.example]", []manifest.Change{{Field: "egress", After: "api.partner.example"}}},
		"a guard": {`access: {"/rooms/{id}": {require: [staff]}}`, `access: {"/rooms/{id}": {require: [staff], guard: true}}`,
			[]manifest.Change{{Field: "access", Key: "/rooms/{id}", Before: "require staff", After: "require staff; guard"}}},
		"a cron": {"schedule: [{cron: '0 6 * * *', job: daily}]", "schedule: [{cron: '0 7 * * *', job: daily}]",
			[]manifest.Change{{Field: "schedule", Key: "daily", Before: "0 6 * * *", After: "0 7 * * *"}}},
		"a data class": {"data: [{source: sqldb, classes: [internal]}]", "data: [{source: sqldb, classes: [internal, finance]}]",
			[]manifest.Change{{Field: "data", Key: "sqldb", Before: "internal", After: "finance, internal"}}},
		"no email": {"email: {from: ops@acme.example, to_domains: [partner.example, acme.example]}", "", []manifest.Change{
			{Field: "email.from", Before: "ops@acme.example"},
			{Field: "email.to_domains", Before: "acme.example"},
			{Field: "email.to_domains", Before: "partner.example"},
		}},
		"a called function": {"services: {calls: {inventory: [ReserveParts]}}", "services: {calls: {inventory: [ReserveParts, Count]}}",
			[]manifest.Change{{Field: "services.calls", Key: "inventory", After: "Count"}}},
		"a size": {"size: S", "size: M", []manifest.Change{{Field: "size", Before: "S", After: "M"}}},
		"entries removed": {"services: {calls: {billing: [Pay]}}\nschedule: [{cron: '0 6 * * *', job: daily}]",
			"audience: {external: [magic-link]}\nmodules: [golang.org/x/text]", []manifest.Change{
				{Field: "audience.external", After: "magic-link"},
				{Field: "services.calls", Key: "billing", Before: "Pay"},
				{Field: "schedule", Key: "daily", Before: "0 6 * * *"},
				{Field: "modules", After: "golang.org/x/text"},
			}},
		"removals first": {"settings: [c, e, a]", "settings: [e, d, b]", []manifest.Change{
			{Field: "settings", Before: "a"}, {Field: "settings", Before: "c"},
			{Field: "settings", After: "b"}, {Field: "settings", After: "d"},
		}},
		"calls and apps": {"", `access: {"/": {require: [staff, admin], calls: [Save, Load]}}` + "\n" +
			"services: {serves: {Get: {callers: [shop, billing], require: [staff, admin]}, Ping: {callers: [shop], apps: true}}}",
			[]manifest.Change{
				{Field: "access", Key: "/", After: "require staff, admin; calls Save, Load"},
				{Field: "services.serves", Key: "Get", After: "callers billing, shop; require admin, staff"},
				{Field: "services.serves", Key: "Ping", After: "callers shop; apps"},
			}},
	} {
		assert.Equal(t, c.want, manifest.Diff(parse(c.before), parse(c.after)), name)
	}
}
