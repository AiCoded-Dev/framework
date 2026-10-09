package gobuf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsts(t *testing.T) {
	b := New()
	b.WritePrintString("<p>\"a\"\n")
	b.WriteStringLn("x()")
	b.WritePrintString("<p>\"a\"\n")
	b.WriteConsts()
	assert.Equal(t, "if _, err := io.WriteString(w, _html0); err != nil {\nreturn err\n}\nx()\n"+
		"if _, err := io.WriteString(w, _html0); err != nil {\nreturn err\n}\nconst (\n_html0 = \"<p>\\\"a\\\"\\n\"\n)\n", b.String())
}

func TestConstsInOrderOfFirstUse(t *testing.T) {
	b := New()
	for _, s := range []string{"b", "a", "b", "c"} {
		b.WritePrintString(s)
		b.WriteStringLn("")
	}
	b.WriteConsts()
	assert.Contains(t, b.String(), "const (\n_html0 = \"b\"\n_html1 = \"a\"\n_html2 = \"c\"\n)\n")
}

func TestNoConsts(t *testing.T) {
	b := New()
	b.WriteStringLn("x()")
	b.WriteConsts()
	assert.Equal(t, "x()\n", b.String())
}

func TestWriteQuotedString(t *testing.T) {
	b := New()
	b.WriteQuotedString("a\\\"\n`", ",")
	assert.Equal(t, "\"a\\\\\\\"\\n`\",", b.String())
}

func TestLineReset(t *testing.T) {
	b := New()
	b.WriteStringLn("package p\n\nfunc f() error {\n//line index.html:3\nx()")
	b.WritePrintString("<p>")
	b.WriteLineReset("route_gen.go")
	b.WriteStringLn("return nil\n}")
	code, err := b.Formatted()
	require.NoError(t, err)
	assert.Equal(t, "package p\n\nfunc f() error {\n//line index.html:3\n\tx()\n//line route_gen.go:7\n"+
		"\tif _, err := io.WriteString(w, _html0); err != nil {\n\t\treturn err\n\t}\n\treturn nil\n}\n", string(code))
}

func TestLineResetStartsItsLine(t *testing.T) {
	b := New()
	b.WriteString("package p\n\nvar x = 1")
	b.WriteLineReset("route_gen.go")
	_, err := b.Formatted()
	assert.EqualError(t, err, "line 3: a line reset must start its line")
}
