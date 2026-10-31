package manifest_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/manifest"
)

const bound = `egress:
  - hooks.partner.example
  - host: maps.partner.example
    secret: partner_api_key
    header: X-Api-Key
  - host: api.partner.example
    secret: partner_api_key
    header: Authorization
    prefix: "Bearer "
secrets: [partner_api_key]`

func TestEgress(t *testing.T) {
	m := load(t, a1(bound))
	api := manifest.Egress{Host: "api.partner.example", Secret: "partner_api_key", Header: "Authorization", Prefix: "Bearer "}
	maps := manifest.Egress{Host: "maps.partner.example", Secret: "partner_api_key", Header: "X-Api-Key"}
	assert.Equal(t, []manifest.Egress{{Host: "hooks.partner.example"}, maps, api}, m.Egress)
	assert.Equal(t, []string{"api.partner.example", "hooks.partner.example", "maps.partner.example"}, m.EgressHosts())

	got, ok := m.Binding("api.partner.example")
	assert.True(t, ok)
	assert.Equal(t, api, got)
	for _, host := range []string{"hooks.partner.example", "other.example", "API.partner.example"} {
		_, ok = m.Binding(host)
		assert.False(t, ok, host)
	}

	got, ok = m.BoundSecret("partner_api_key")
	assert.True(t, ok)
	assert.Equal(t, api, got, "the first binding by host")
	_, ok = m.BoundSecret("other_key")
	assert.False(t, ok)

	assert.True(t, api.Bound())
	assert.False(t, manifest.Egress{Host: "hooks.partner.example"}.Bound())
	assert.Equal(t, []manifest.Egress{{Host: "a.example"}}, load(t, a1("egress: [{host: a.example}]")).Egress, "a host alone is plain")
}

func TestPlatformHost(t *testing.T) {
	for _, host := range []string{"aicoded.cloud", "api.aicoded.cloud", "aicoded.dev", "docs.aicoded.dev", "API.AICODED.cloud", "a.b.aicoded.cloud."} {
		assert.True(t, manifest.PlatformHost(host), host)
	}
	for _, host := range []string{"aicoded.cloud.example", "notaicoded.cloud", "xaicoded.dev", "aicoded.devs", "api.partner.example"} {
		assert.False(t, manifest.PlatformHost(host), host)
	}
}

func TestEgressErrors(t *testing.T) {
	const fixHost = "write the host alone, lower-case, such as api.partner.example: no scheme, port, path, wildcard or IP address, nothing local, each once"
	const fixKey = `bind a declared secret to a header such as Authorization, with an optional prefix such as "Bearer "`
	binding := func(fields string) string {
		return "egress:\n  - host: api.partner.example\n" + fields + "secrets: [k]"
	}
	for src, want := range map[string]struct{ pos, code, msg, fix string }{
		"egress: [api.aicoded.cloud]": {"2", "E-MAN-018", `egress entry "api.aicoded.cloud" is a host of the platform, which an app may not reach`, fixHost},
		"egress: [aicoded.dev]":       {"2", "E-MAN-018", `egress entry "aicoded.dev" is a host of the platform, which an app may not reach`, fixHost},
		"egress: [AICODED.dev]":       {"2", "E-MAN-018", `egress entry "AICODED.dev" is a host of the platform, which an app may not reach`, fixHost},
		"egress:\n  - {host: x.aicoded.cloud, secret: k, header: Authorization}\nsecrets: [k]": {"3", "E-MAN-018",
			`egress entry "x.aicoded.cloud" is a host of the platform, which an app may not reach`, fixHost},
		"egress:\n  - {host: 'https://a.example', secret: k, header: Authorization}\nsecrets: [k]": {"3", "E-MAN-018",
			`egress entry "https://a.example" is not a host name`, fixHost},
		"egress:\n  - a.example\n  - {host: a.example, secret: k, header: Authorization}\nsecrets: [k]": {"4", "E-MAN-018",
			`egress entry "a.example" is listed twice`, fixHost},
		"egress:\n  - {host: a.example, secret: k, header: Authorization}\n  - a.example\nsecrets: [k]": {"4", "E-MAN-018",
			`egress entry "a.example" is listed twice`, fixHost},
		"egress:\n  - {secret: k, header: Authorization}\nsecrets: [k]": {"3", "E-MAN-018", "egress entry has no host", fixHost},

		"egress:\n  - host: api.partner.example\n    secret: k\n    header: Authorization": {"4", "E-MAN-025",
			`egress entry "api.partner.example" binds the secret "k", which secrets does not declare`, fixKey},
		binding("    secret: k\n    header: Auth orization\n"): {"5", "E-MAN-025",
			`egress entry "api.partner.example" has the header "Auth orization", which is not a header name of at most 64 characters`, fixKey},
		binding("    secret: k\n    header: " + strings.Repeat("X", 65) + "\n"): {"5", "E-MAN-025",
			`egress entry "api.partner.example" has the header "` + strings.Repeat("X", 65) + `", which is not a header name of at most 64 characters`, fixKey},
		binding("    secret: k\n    header: cookie\n"): {"5", "E-MAN-025",
			`egress entry "api.partner.example" has the header "cookie", which may not carry a key`, fixKey},
		binding("    secret: k\n    header: \"\"\n"): {"3", "E-MAN-025",
			`egress entry "api.partner.example" binds the secret "k" to no header`, fixKey},
		binding("    header: Authorization\n"): {"3", "E-MAN-025",
			`egress entry "api.partner.example" has a header or a prefix but no secret`, fixKey},
		binding("    prefix: 'Bearer '\n"): {"3", "E-MAN-025",
			`egress entry "api.partner.example" has a header or a prefix but no secret`, fixKey},
		binding("    secret: k\n    header: Authorization\n    prefix: \"Bearer\\t\"\n"): {"6", "E-MAN-025",
			`egress entry "api.partner.example" has a prefix that is not at most 64 printable ASCII characters`, fixKey},
		binding("    secret: k\n    header: Authorization\n    prefix: \"" + strings.Repeat("a", 65) + "\"\n"): {"6", "E-MAN-025",
			`egress entry "api.partner.example" has a prefix that is not at most 64 printable ASCII characters`, fixKey},
		binding("    secret: k\n    header: Authorization\n    prefix: \"Bearér \"\n"): {"6", "E-MAN-025",
			`egress entry "api.partner.example" has a prefix that is not at most 64 printable ASCII characters`, fixKey},
	} {
		e := problem(t, a1(src))
		assert.Equal(t, []string{manifest.FileName + ":" + want.pos, want.code, want.msg, want.fix}, []string{e.Pos, e.Code, e.Msg, e.Fix}, src)
	}
}

func TestEgressHeaders(t *testing.T) {
	src := func(header string) string {
		return a1("egress: [{host: a.example, secret: k, header: '" + header + "'}]\nsecrets: [k]")
	}
	for _, header := range []string{"Authorization", "X-Api-Key", "api_key", "x!#$%&*+.^`|~y", strings.Repeat("X", 64)} {
		_, err := manifest.Parse(manifest.FileName, []byte(src(header)))
		require.NoError(t, err, header)
	}
	for _, header := range []string{"Connection", "keep-alive", "Proxy-Authenticate", "PROXY-AUTHORIZATION", "Proxy-Connection", "te",
		"Trailer", "Transfer-Encoding", "Upgrade", "Host", "Content-Length", "content-type", "Cookie",
		"X:Y", "X(Y)", "X\"Y", "Ä", "X/Y", "X@Y", "X=Y"} {
		assert.Equal(t, "E-MAN-025", problem(t, src(header)).Code, header)
	}
}

func TestEgressUnknownKey(t *testing.T) {
	e := problem(t, a1("egress:\n  - host: a.example\n    token: x"))
	assert.Equal(t, []string{manifest.FileName + ":4", "E-MAN-014", `unknown key "token" in egress`,
		"remove it, or correct its name: the keys there are host, secret, header, prefix"}, []string{e.Pos, e.Code, e.Msg, e.Fix})
}

func TestEgressShape(t *testing.T) {
	for _, src := range []string{"egress: [[a.example]]", "egress: [{host: [a.example]}]", "egress: [{host: a.example, secret: {k: v}}]"} {
		assert.Equal(t, []string{"aicoded.yaml:2 E-MAN-003"}, problems(t, a1(src)), src)
	}
}
