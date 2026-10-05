package web

import (
	"html"
	"strings"
)

// SafeHTML is markup that a template writes as is with {{$ expr }}. {{ expr }} escapes it
// like any other value.
type SafeHTML struct{ s string }

// stringConstant is unexported so that callers cannot name it.
type stringConstant string

// HTMLConst returns markup written in the source code. Call it directly with a string
// constant: a string variable passed to it does not compile.
func HTMLConst(markup stringConstant) SafeHTML { return SafeHTML{string(markup)} }

// EscapeHTML returns markup that shows exactly text.
func EscapeHTML(text string) SafeHTML { return SafeHTML{html.EscapeString(text)} }

// JoinHTML concatenates markup.
func JoinHTML(parts ...SafeHTML) SafeHTML {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p.s)
	}
	return SafeHTML{b.String()}
}

// String returns the markup.
func (h SafeHTML) String() string { return h.s }
