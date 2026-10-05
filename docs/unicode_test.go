package docs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoHiddenUnicode(t *testing.T) {
	root := ".."
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if d.IsDir() || !scanned[filepath.Ext(path)] {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for _, p := range hidden(filepath.ToSlash(rel), string(data)) {
			t.Error(p)
		}
		return nil
	})
	require.NoError(t, err)
}

func TestHidden(t *testing.T) {
	text := "# A\n\nsafe \u2014 text\u2026 caf\u00e9\nif admin\u202e {\n"
	assert.Equal(t, []string{"a.md:4: hidden character U+202E; delete it"}, hidden("a.md", text))

	for _, c := range []string{
		"\u00AD", "\u034F", "\u061C", "\u180E", "\u200B", "\u200F", "\u2028", "\u2029", "\u202A", "\u202E",
		"\u2060", "\u2064", "\u2066", "\u2069", "\uFEFF", "\U000E0000", "\U000E0041", "\U000E007F",
	} {
		assert.Len(t, hidden("a.md", "x"+c), 1, "%+q", c)
	}
	for _, c := range []string{
		"\u00A0", "\u034E", "\u0350", "\u2010", "\u2014", "\u2026", "\u2027", "\u2065", "\u206A", "\U000DFFFF", "\U000E0080",
	} {
		assert.Empty(t, hidden("a.md", "x"+c), "%+q", c)
	}
}

// scanned are the extensions of the files that TestNoHiddenUnicode reads.
var scanned = map[string]bool{
	".md": true, ".go": true, ".html": true, ".ts": true, ".css": true,
	".yaml": true, ".yml": true, ".json": true, ".txt": true, ".mod": true,
}

// isHidden reports whether r is a character that does not show, or that reorders or breaks the
// text around it: the soft hyphen, the combining grapheme joiner, the Arabic letter mark, the
// Mongolian vowel separator, zero-width characters and marks, the line and paragraph separators,
// bidi embeddings, overrides and isolates, invisible operators, the byte order mark, and the tags,
// which can hide text from a reader but not from an AI assistant.
func isHidden(r rune) bool {
	switch {
	case r == 0x00AD, r == 0x034F, r == 0x061C, r == 0x180E, r == 0xFEFF,
		r >= 0x200B && r <= 0x200F,
		r >= 0x2028 && r <= 0x202E,
		r >= 0x2060 && r <= 0x2064,
		r >= 0x2066 && r <= 0x2069,
		r >= 0xE0000 && r <= 0xE007F:
		return true
	}
	return false
}

// hidden returns a problem for every hidden character of text, the content of the file rel.
func hidden(rel, text string) []string {
	var problems []string
	for i, line := range strings.Split(text, "\n") {
		for _, r := range line {
			if isHidden(r) {
				problems = append(problems, fmt.Sprintf("%s:%d: hidden character U+%04X; delete it", rel, i+1, r))
			}
		}
	}
	return problems
}
