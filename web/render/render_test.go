package render

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/web"
)

func write(t *testing.T, f func(*strings.Builder) error) string {
	t.Helper()
	var b strings.Builder
	require.NoError(t, f(&b))
	return b.String()
}

func TestFormat(t *testing.T) {
	for v, want := range map[any]string{
		nil: "", "s": "s", true: "true", 42: "42", int8(-3): "-3", int16(5): "5", int32(6): "6", int64(7): "7",
		uint(8): "8", uint8(9): "9", uint16(10): "10", uint32(11): "11", uint64(12): "12",
		1.5: "1.5", float32(0.1): "0.1", web.HTMLConst("<b>"): "<b>",
	} {
		assert.Equal(t, want, Format(v), "%T %v", v, v)
	}
}

func TestTextAndAttrEscape(t *testing.T) {
	for in, want := range map[string]string{
		`<script>alert("x")</script>`:    "&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;",
		`"><img src=x onerror=alert(1)>`: "&#34;&gt;&lt;img src=x onerror=alert(1)&gt;",
		"a & 'b'":                        "a &amp; &#39;b&#39;",
		"nul\x00":                        "nul�",
		"plain":                          "plain",
	} {
		assert.Equal(t, want, write(t, func(b *strings.Builder) error { return Text(b, in) }))
		assert.Equal(t, want, write(t, func(b *strings.Builder) error { return Attr(b, in) }))
	}
	assert.Equal(t, "&lt;b&gt;", write(t, func(b *strings.Builder) error { return Text(b, web.HTMLConst("<b>")) }))
}

func TestURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://example.com/a?b=c&d=e": "https://example.com/a?b=c&amp;d=e",
		"/users/42":                     "/users/42",
		"mailto:a@b.example":            "mailto:a@b.example",
		"HTTP://X.example":              "HTTP://X.example",
		"page?x=1":                      "page?x=1",
		"//cdn.example/x":               "//cdn.example/x",
		"/a:b":                          "/a:b",
		`/a"><script>`:                  "/a%22%3e%3cscript%3e",
		"/a b":                          "/a%20b",
		"javascript:alert(1)":           UnsafeURL,
		"JaVaScRiPt:alert(1)":           UnsafeURL,
		" javascript:alert(1)":          UnsafeURL,
		"java\tscript:alert(1)":         UnsafeURL,
		"data:text/html,<script>":       UnsafeURL,
		"vbscript:x":                    UnsafeURL,
		"a:b":                           UnsafeURL,
	} {
		assert.Equal(t, want, write(t, func(b *strings.Builder) error { return URL(b, in) }), in)
	}
}

func TestURLParts(t *testing.T) {
	assert.Equal(t, "a%20b/c?d&amp;e", write(t, func(b *strings.Builder) error { return URLPart(b, "a b/c?d&e") }))
	assert.Equal(t, "javascript:x", write(t, func(b *strings.Builder) error { return URLPart(b, "javascript:x") }))
	assert.Equal(t, "a%20b%26c%3dd%2f%c3%a9", write(t, func(b *strings.Builder) error { return URLQuery(b, "a b&c=d/é") }))
	assert.Equal(t, "/evil.example", write(t, func(b *strings.Builder) error { return URLPart(b, "/evil.example") }))
}

// A value right after a URL's leading "/" never makes the URL start with "//" or "/\", which
// browsers read as another site.
func TestURLPathStart(t *testing.T) {
	for in, want := range map[string]string{
		"notes/1":        "notes/1",
		"a b?c":          "a%20b?c",
		"/evil.example":  "%2fevil.example",
		"//evil.example": "%2f/evil.example",
		`\evil.example`:  "%5cevil.example",
		"\t/evil":        "%09/evil",
		"":               ".",
	} {
		assert.Equal(t, want, write(t, func(b *strings.Builder) error { return URLPathStart(b, in) }), in)
	}
}

func TestJSON(t *testing.T) {
	out := write(t, func(b *strings.Builder) error { return JSON(b, "</script><!--&\u2028") })
	assert.Equal(t, `"\u003c/script\u003e\u003c!--\u0026\u2028"`, out)
	var b strings.Builder
	assert.Error(t, JSON(&b, func() {}))
}

func TestHTMLAndIf(t *testing.T) {
	assert.Equal(t, "<b>x</b>", write(t, func(b *strings.Builder) error { return HTML(b, web.HTMLConst("<b>x</b>")) }))
	assert.Equal(t, "a", If(true, "a", "b"))
	assert.Equal(t, 2, If(false, 1, 2))
}
