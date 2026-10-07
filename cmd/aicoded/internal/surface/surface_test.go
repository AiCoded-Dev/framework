package surface

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"aicoded.dev/framework/lint"
	"aicoded.dev/framework/manifest"
)

func TestOf(t *testing.T) {
	m := manifest.Manifest{
		Access: map[string]manifest.Access{
			"/":           {Require: []string{"*"}, Calls: []string{"count"}},
			"/notes/{id}": {Require: []string{"*"}, Calls: []string{"count", "star"}},
		},
		Services: manifest.Services{Serves: map[string]manifest.Serve{"Notes": {Callers: []string{"desk"}}}},
		Email:    &manifest.Email{From: "notes@acme.example", ToDomains: []string{"acme.example"}},
	}
	s := Of(m, []lint.Write{{Table: "notes", Op: "INSERT"}, {Table: "notes", Op: "UPDATE"}})
	assert.Equal(t, Surface{Size: 6, EntryPoints: 6, Effects: 3, Writes: []string{"INSERT notes", "UPDATE notes"}}, s,
		"2 pages, 3 page calls and 1 function; 2 writes and 1 mail domain")
	assert.Equal(t, "surface 6 (6 entry points, 3 effects)", s.String())

	m.Schedule = []manifest.Job{{Cron: "0 6 * * *", Name: "daily-summary"}, {Cron: "0 * * * *", Name: "sync"}}
	m.Data = []manifest.Data{{Source: "sqldb"}, {Source: "filestore:photos"}, {Source: "connector:crm"}}
	m.Egress = []string{"api.partner.example", "hooks.partner.example"}
	s = Of(m, []lint.Write{{Table: "notes", Op: "INSERT"}, {Table: "notes", Op: "UPDATE"}})
	assert.Equal(t, Surface{Size: 8, EntryPoints: 8, Effects: 6, Writes: []string{"INSERT notes", "UPDATE notes"}}, s,
		"2 more entry points for the jobs; 3 more effects for the connector and the hosts")

	s = Of(manifest.Manifest{}, []lint.Write{{Table: "notes", Op: "DELETE"}})
	assert.Equal(t, Surface{Size: 1, Effects: 1, Writes: []string{"DELETE notes"}}, s, "effects can be the larger")
	assert.Equal(t, "surface 1 (0 entry points, 1 effect)", s.String())
}
