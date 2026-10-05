package errs

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var codeRe = regexp.MustCompile(`"(E-[A-Z]+-[0-9]{3})"`)

// Every code used outside tests has a page in docs/errors.
func TestCatalogue(t *testing.T) {
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != root && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range codeRe.FindAllStringSubmatch(string(src), -1) {
			assert.FileExists(t, filepath.Join(root, "docs", "errors", m[1]+".md"), "%s uses %s", path, m[1])
		}
		return nil
	})
	require.NoError(t, err)
}
