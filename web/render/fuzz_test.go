package render

import (
	"html"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func FuzzAttr(f *testing.F) {
	f.Add(`"><script>alert(1)</script>`)
	f.Add("&amp;\x00'")
	f.Fuzz(func(t *testing.T, s string) {
		var b strings.Builder
		require.NoError(t, Attr(&b, s))
		out := b.String()
		assert.NotContains(t, out, `"`)
		assert.NotContains(t, out, "'")
		assert.NotContains(t, out, "<")
		assert.NotContains(t, out, ">")
		assert.Equal(t, strings.ReplaceAll(s, "\x00", "�"), html.UnescapeString(out))
	})
}

func FuzzURL(f *testing.F) {
	for _, s := range []string{"javascript:alert(1)", "https://x.example/?a=b", " java\nscript:x", "/p"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		var b strings.Builder
		require.NoError(t, URL(&b, s))
		out := html.UnescapeString(b.String())
		for i := 0; i < len(out); i++ {
			assert.True(t, out[i] > 0x20 && out[i] < 0x7f, "byte %#x in %q", out[i], out)
		}
		assert.True(t, out == UnsafeURL || allowedScheme(out), out)
	})
}

func FuzzURLPathStart(f *testing.F) {
	for _, s := range []string{"/evil.example", `\\evil`, "", "notes/1"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		var b strings.Builder
		require.NoError(t, URLPathStart(&b, s))
		out := html.UnescapeString(b.String())
		require.NotEmpty(t, out)
		assert.False(t, out[0] == '/' || out[0] == '\\', "the URL /%s starts with two slashes", out)
	})
}
