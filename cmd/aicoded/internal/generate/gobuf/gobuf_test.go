package gobuf

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
