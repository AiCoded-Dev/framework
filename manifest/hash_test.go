package manifest_test

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"aicoded.dev/framework/manifest"
)

const ordered = `app: notes
class: internal-tool
owner: group:ops
audience:
  internal: [everyone, group:ops]
  external: [idp:partners, magic-link]
data:
  - source: filestore:docs
    classes: [internal, personal]
  - source: sqldb
    classes: [confidential, internal]
access:
  "/a":
    require: ["admin|owner", "staff"]
    calls: [star, unstar]
  "/b/{id}":
    require: ["*"]
    guard: true
  "/c/{id}":
    require: ["*"]
    shared: true
services:
  calls:
    billing: [GetInvoice, Ping]
  serves:
    Order:
      callers: [reports, shop]
      require: ["staff"]
egress: [a.example, b.example]
email:
  from: ops@acme.example
  to_domains: [acme.example, partner.example]
schedule:
  - {cron: "0 6 * * *", job: daily}
  - {cron: "0 7 * * 1", job: weekly}
settings: [day, greeting]
secrets: [api_key, token]
modules: [golang.org/x/net, golang.org/x/text]
size: S
resources: small
ttl: 180d
`

const shuffled = `# the same list, written in another order
ttl: 180d
resources: small
size: S
modules: [golang.org/x/text, golang.org/x/net]
secrets: [token, api_key]
settings: [greeting, day]
schedule:
  - {job: weekly, cron: "0 7 * * 1"}
  - {cron: "0 6 * * *", job: daily}
email:
  to_domains: [partner.example, acme.example]  # two domains
  from: ops@acme.example
egress: [b.example, a.example]
services:
  serves:
    Order:
      require: ["staff"]
      callers: [shop, reports]
  calls:
    billing: [Ping, GetInvoice]
access:
  "/c/{id}":
    shared: true
    require: ["*"]
  "/b/{id}":
    guard: true
    require: ["*"]
  "/a":
    calls: [unstar, star]
    require: ["staff", "admin|owner"]
data:
  - source: sqldb
    classes: [internal, confidential]
  - classes: [personal, internal]
    source: filestore:docs
audience:
  external: [magic-link, idp:partners]
  internal: [group:ops, everyone]
owner: group:ops
class: internal-tool
app: notes
`

func TestHashIgnoresOrder(t *testing.T) {
	a, b := load(t, ordered), load(t, shuffled)
	assert.Equal(t, manifest.Hash(a), manifest.Hash(b))
	assert.Equal(t, string(manifest.Canonical(a)), string(manifest.Canonical(b)))
	assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, manifest.Hash(a))

	for _, change := range [][2]string{
		{"app: notes", "app: notes2"},
		{"class: internal-tool", "class: other"},
		{"owner: group:ops", "owner: group:hr"},
		{"internal: [everyone, group:ops]", "internal: [everyone]"},
		{"external: [idp:partners, magic-link]", "external: [magic-link]"},
		{"source: filestore:docs", "source: filestore:files"},
		{"classes: [internal, personal]", "classes: [internal]"},
		{`require: ["admin|owner", "staff"]`, `require: ["admin", "staff"]`},
		{"calls: [star, unstar]", "calls: [star]"},
		{"    guard: true", "    shared: true"},
		{"    shared: true\n", ""},
		{"billing: [GetInvoice, Ping]", "billing: [Ping]"},
		{"callers: [reports, shop]", "callers: [shop]"},
		{`      require: ["staff"]`, `      require: ["admin"]`},
		{"      require: [\"staff\"]\n", "      require: [\"staff\"]\n      apps: true\n"},
		{"egress: [a.example, b.example]", "egress: [a.example]"},
		{"from: ops@acme.example", "from: desk@acme.example"},
		{"to_domains: [acme.example, partner.example]", "to_domains: [acme.example]"},
		{`{cron: "0 6 * * *", job: daily}`, `{cron: "0 5 * * *", job: daily}`},
		{"settings: [day, greeting]", "settings: [day]"},
		{"secrets: [api_key, token]", "secrets: [api_key]"},
		{"modules: [golang.org/x/net, golang.org/x/text]", "modules: [golang.org/x/net]"},
		{"size: S", "size: M"},
		{"resources: small", "resources: large"},
		{"ttl: 180d", "ttl: 90d"},
	} {
		changed := strings.Replace(ordered, change[0], change[1], 1)
		assert.NotEqual(t, ordered, changed, change[0])
		assert.NotEqual(t, manifest.Hash(a), manifest.Hash(load(t, changed)), change[0])
	}
}

func TestCanonicalSets(t *testing.T) {
	m := manifest.Manifest{App: "a1", Egress: []manifest.Egress{{Host: "b.example"}, {Host: "a.example"}, {Host: "b.example"}}, Settings: []string{}, Access: map[string]manifest.Access{}}
	const want = `{"app":"a1","egress":["a.example","b.example"]}`
	assert.JSONEq(t, want, string(manifest.Canonical(m)), "every list is a set, and what is empty is not stated")
	sum := sha256.Sum256([]byte(want))
	assert.Equal(t, "sha256:"+hex.EncodeToString(sum[:]), manifest.Hash(m), "the hash is of the canonical form, byte for byte")
	assert.Equal(t, []manifest.Egress{{Host: "b.example"}, {Host: "a.example"}, {Host: "b.example"}}, m.Egress, "the manifest is not changed")
}

// Permission lists without egress, or with plain hosts only, hash as they did before keys could
// be bound to hosts.
func TestHashBeforeBindings(t *testing.T) {
	assert.Equal(t, "sha256:e0e32e6a80beec0584fbeeddad346e3817de2f85dc38700f3cc16b1f9e8bd6b4", manifest.Hash(load(t, ordered)))
	noEgress := strings.Replace(ordered, "egress: [a.example, b.example]\n", "", 1)
	assert.Equal(t, "sha256:16614a4fc2c28cab9a0426bc307e20179e2cefe05496c604e249208a43d10805", manifest.Hash(load(t, noEgress)))
}

func TestCanonicalBindings(t *testing.T) {
	key := manifest.Egress{Host: "b.example", Secret: "k", Header: "Authorization", Prefix: "Bearer "}
	m := manifest.Manifest{App: "a1", Egress: []manifest.Egress{{Host: "c.example"}, key, {Host: "a.example", Secret: "k", Header: "X-Key"}, key}}
	const want = `{"app":"a1","egress":[{"host":"a.example","secret":"k","header":"X-Key"},` +
		`{"host":"b.example","secret":"k","header":"Authorization","prefix":"Bearer "},"c.example"]}`
	assert.JSONEq(t, want, string(manifest.Canonical(m)))
	sum := sha256.Sum256([]byte(want))
	assert.Equal(t, "sha256:"+hex.EncodeToString(sum[:]), manifest.Hash(m), "plain hosts and bindings sorted by host, byte for byte")

	for name, e := range map[string]manifest.Egress{
		"plain":  {Host: "b.example"},
		"secret": {Host: "b.example", Secret: "j", Header: "Authorization", Prefix: "Bearer "},
		"header": {Host: "b.example", Secret: "k", Header: "X-Key", Prefix: "Bearer "},
		"prefix": {Host: "b.example", Secret: "k", Header: "Authorization"},
		"host":   {Host: "d.example", Secret: "k", Header: "Authorization", Prefix: "Bearer "},
	} {
		other := manifest.Manifest{App: "a1", Egress: []manifest.Egress{{Host: "c.example"}, e, {Host: "a.example", Secret: "k", Header: "X-Key"}}}
		assert.NotEqual(t, manifest.Hash(m), manifest.Hash(other), name)
	}
}
