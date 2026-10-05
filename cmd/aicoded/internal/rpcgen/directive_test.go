package rpcgen

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAccess(t *testing.T) {
	for text, want := range map[string]Access{
		"//ssr:access caller=shop role=editor|admin":             {Callers: []string{"shop"}, Require: []string{"admin|editor"}},
		"//ssr:access caller=shop role=*":                        {Callers: []string{"shop"}, Require: []string{"*"}},
		"//ssr:access role=x caller=shop,reports,shop apps=true": {Callers: []string{"reports", "shop"}, Require: []string{"x"}, Apps: true},
		"//ssr:access caller=shop apps=true":                     {Callers: []string{"shop"}, Apps: true},
		"//ssr:access\tcaller=shop  role=a|a":                    {Callers: []string{"shop"}, Require: []string{"a"}},
	} {
		a, err := parseAccess(text)
		require.NoError(t, err, text)
		assert.Equal(t, want, a, text)
	}
}

func TestParseAccessRefuses(t *testing.T) {
	for text, want := range map[string]string{
		"//ssr:access caller=shop role=x mode=fast": "mode= is not an attribute",
		"//ssr:access caller=shop role=x role=y":    "role= is given twice",
		"//ssr:access caller=shop role":             `"role" is not an attribute`,
		"//ssr:access role=x":                       "caller= is missing",
		"//ssr:access caller=Shop role=x":           `caller "Shop" is not an app name`,
		"//ssr:access caller=shop, role=x":          `caller "" is not an app name`,
		"//ssr:access caller=shop role=Admin":       `role "Admin" is not a lower-case name`,
		"//ssr:access caller=shop role=":            `role "" is not a lower-case name`,
		"//ssr:access caller=shop role=*|admin":     `"*" admits every viewer, so it stands alone`,
		"//ssr:access caller=shop":                  "role= and apps=true are both missing",
		"//ssr:access caller=shop apps=false":       "write apps=true or leave it out",
	} {
		_, err := parseAccess(text)
		require.ErrorContains(t, err, want, text)
	}
}

func TestIsDirective(t *testing.T) {
	assert.True(t, isDirective("//ssr:access caller=a role=*"))
	assert.True(t, isDirective("//ssr:access"))
	assert.False(t, isDirective("//ssr:accessible"))
	assert.False(t, isDirective("// ssr:access caller=a role=*"))
}

func TestIsSpaced(t *testing.T) {
	assert.True(t, isSpaced("// ssr:access caller=a role=*"))
	assert.True(t, isSpaced("//\t ssr:access"))
	assert.False(t, isSpaced("//ssr:access caller=a role=*"))
	assert.False(t, isSpaced("// ssr:accessible"))
	assert.False(t, isSpaced("// see ssr:access"))
}
