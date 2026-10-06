package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/platform"
	"aicoded.dev/framework/cmd/aicoded/internal/platform/platformtest"
	"aicoded.dev/framework/cmd/aicoded/internal/testhome"
)

// fakePlatform starts a fake platform for aicoded, in a home of its own, and returns a function
// that runs aicoded with args and returns its exit code, output and errors. A browser signs in at
// each address of a sign-in that aicoded prints. No token may appear in the output.
func fakePlatform(t *testing.T) (*platformtest.Platform, func(args ...string) (int, string, string)) {
	testhome.Set(t)
	p := platformtest.New(t)
	t.Setenv("AICODED_PLATFORM", p.URL)
	browse, _ := platformtest.Browser()
	return p, func(args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		code := run(args, platformtest.Watch(&out, browse), &errOut)
		assert.NotRegexp(t, `aicoded_(at|rt|ac|dc)_`, out.String()+errOut.String(), "a token was printed")
		return code, out.String(), errOut.String()
	}
}

func TestRunLoginWhoamiLogout(t *testing.T) {
	p, aicoded := fakePlatform(t)

	code, out, _ := aicoded("login", "--org", "acme")
	require.Equal(t, 0, code)
	assert.Contains(t, out, "\n    "+p.URL+"/oauth/authorize?")
	assert.Contains(t, out, "\nSigned in to acme as ana@acme.example.\n")

	code, out, _ = aicoded("whoami")
	assert.Equal(t, 0, code)
	assert.Equal(t, "org: acme\nemail: ana@acme.example\nscopes: app:create\n", out)
	code, out, _ = aicoded("whoami", "--json")
	assert.Equal(t, 0, code)
	assert.JSONEq(t, `{"org": "acme", "email": "ana@acme.example", "scopes": ["app:create"]}`, out)

	code, out, _ = aicoded("logout")
	assert.Equal(t, 0, code)
	assert.Equal(t, "Signed out of "+p.URL+".\n", out)
	assert.Len(t, p.Revoked(), 1)
	code, _, errOut := aicoded("whoami")
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "E-CLI-004: not signed in to the platform at "+p.URL)
	code, out, _ = aicoded("logout")
	assert.Equal(t, 0, code)
	assert.Equal(t, "Not signed in to "+p.URL+".\n", out)
}

func TestRunLogoutKeepsTheOrg(t *testing.T) {
	p, aicoded := fakePlatform(t)
	code, _, _ := aicoded("login", "--org", "acme")
	require.Equal(t, 0, code)
	code, _, _ = aicoded("logout")
	require.Equal(t, 0, code)
	cr, err := platform.Load(p.URL)
	require.NoError(t, err)
	assert.Equal(t, platform.Credentials{Org: "acme"}, cr, "no token is kept")

	code, out, errOut := aicoded("login")
	require.Equal(t, 0, code, errOut)
	assert.Contains(t, out, "\nSigned in to acme as ana@acme.example.\n")
}

func TestRunLoginWithACode(t *testing.T) {
	_, aicoded := fakePlatform(t)
	code, out, errOut := aicoded("login", "--device", "--org", "acme")
	require.Equal(t, 0, code, errOut)
	assert.Contains(t, out, "and enter the code BCDF-GHJK\n")
	assert.Contains(t, out, "\nSigned in to acme as ana@acme.example.\n")
}

func TestRunLoginUsage(t *testing.T) {
	_, aicoded := fakePlatform(t)
	code, _, errOut := aicoded("login")
	assert.Equal(t, 2, code)
	assert.Contains(t, errOut, "aicoded login: the first login needs --org <organisation>\n")
	code, _, errOut = aicoded("login", "--org", "Acme Corp")
	assert.Equal(t, 2, code)
	assert.Contains(t, errOut, "aicoded login: --org must name an organisation: 2 to 39 lowercase letters, digits and dashes\n")
	code, _, _ = aicoded("login", "acme")
	assert.Equal(t, 2, code)
	code, _, _ = aicoded("whoami", "me")
	assert.Equal(t, 2, code)
	code, _, _ = aicoded("logout", "--all")
	assert.Equal(t, 2, code)
}

func TestRunLogoutWithoutThePlatform(t *testing.T) {
	p, aicoded := fakePlatform(t)
	code, _, _ := aicoded("login", "--org", "acme")
	require.Equal(t, 0, code)
	p.Close()
	code, out, _ := aicoded("logout")
	assert.Equal(t, 0, code)
	assert.Equal(t, "Signed out of "+p.URL+" on this computer, but the platform did not revoke the sign-in: "+
		"it stays valid there until it expires, within 30 days.\n", out)
	code, _, errOut := aicoded("whoami")
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "E-CLI-004")
}

func TestRunPlatformAddress(t *testing.T) {
	_, aicoded := fakePlatform(t)
	t.Setenv("AICODED_PLATFORM", "http://api.acme.example")
	for _, args := range [][]string{{"login", "--org", "acme"}, {"logout"}, {"whoami"}} {
		code, _, errOut := aicoded(args...)
		assert.Equal(t, 1, code, args)
		assert.Contains(t, errOut, "E-CLI-005", args)
	}
}
