package manifest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/manifest"
)

func FuzzParse(f *testing.F) {
	full, err := os.ReadFile(filepath.Join("testdata", "full.yaml"))
	require.NoError(f, err)
	for _, seed := range []string{
		string(full),
		"",
		"app: [\n",
		"app: a1\ncolour: red\negress: [localhost]\nschedule: [{cron: '0 25 * * *', job: Daily}]\nttl: 6m\n",
		"app: a1\nsettings: &a [x]\nsecrets: *a\nemail:\n  <<: {from: a@acme.example}\n",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := manifest.Parse(manifest.FileName, data)
		if err != nil {
			assert.NotEmpty(t, errs.Code(err), "every problem is coded")
			return
		}
		again, err := manifest.Parse(manifest.FileName, data)
		require.NoError(t, err)
		assert.Equal(t, m, again)
	})
}
