package web

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/web/reactive"
)

func TestSafeHTML(t *testing.T) {
	assert.Equal(t, "<b>hi</b>", HTMLConst("<b>hi</b>").String())
	assert.Equal(t, "&lt;b&gt;&#34;x&#34; &amp; &#39;y&#39;", EscapeHTML(`<b>"x" & 'y'`).String())
	assert.Equal(t, "<i>a</i>&lt;b&gt;", JoinHTML(HTMLConst("<i>a</i>"), EscapeHTML("<b>")).String())
	assert.Empty(t, SafeHTML{}.String())
}

// A string variable passed directly to HTMLConst does not compile.
func TestHTMLConstRejectsVariables(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	out, err := exec.CommandContext(t.Context(), "go", "build", "-o", os.DevNull, "./testdata/htmlvar").CombinedOutput()
	require.Error(t, err)
	assert.Contains(t, string(out), "stringConstant")
}

// A page cannot write markup into a live variable of type SafeHTML: its value never decodes
// from what the page sends.
func TestSafeHTMLDoesNotDecode(t *testing.T) {
	_, err := reactive.Decode[SafeHTML]([]byte(`"<img src=x onerror=alert(1)>"`))
	require.Error(t, err)
	h, err := reactive.Decode[SafeHTML]([]byte(`{"s":"<img src=x onerror=alert(1)>"}`))
	require.NoError(t, err)
	assert.Empty(t, h.String())
}
