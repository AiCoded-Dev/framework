package platform_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/platform"
	"aicoded.dev/framework/cmd/aicoded/internal/platform/platformtest"
	"aicoded.dev/framework/cmd/aicoded/internal/testhome"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/migrate"
)

// step is the step seq of a list: the statement stmt, which the publish pub added as its index-th.
func step(seq int, pub string, index int, stmt string) migrate.Step {
	sum := sha256.Sum256([]byte(stmt))
	return migrate.Step{Seq: seq, Publish: pub, Index: index, SHA256: hex.EncodeToString(sum[:]), Statement: stmt}
}

func TestMigrations(t *testing.T) {
	p, c := signedIn(t)
	steps, err := c.Migrations(t.Context(), "demo")
	require.NoError(t, err)
	assert.Empty(t, steps, "an app the platform does not have")

	_, err = c.CreatePublish(t.Context(), upload(sha))
	require.NoError(t, err)
	want := []migrate.Step{step(1, "pub_aaaaaaaaaaaaaaaaaaaaaaaaaa", 1, "CREATE TABLE a (x INT)")}
	p.SetMigrations("demo", want...)
	steps, err = c.Migrations(t.Context(), "demo")
	require.NoError(t, err)
	assert.Equal(t, want, steps)

	p.AddApp("theirs", "bo@acme.example")
	_, err = c.Migrations(t.Context(), "theirs")
	assert.Equal(t, "E-PUB-005", errs.Code(err))
}

// Without a sign-in, Migrations asks nothing and takes no lock, so a configuration folder that
// others can open, or that cannot be written, does not matter.
func TestMigrationsWithoutASignIn(t *testing.T) {
	dir := filepath.Join(testhome.Set(t), ".config", "aicoded")
	p := platformtest.New(t)
	c := platform.New(p.URL, &bytes.Buffer{})
	for _, mode := range []os.FileMode{0o755, 0o500} {
		require.NoError(t, os.MkdirAll(dir, 0o700))
		require.NoError(t, os.Chmod(dir, mode))
		_, err := c.Migrations(t.Context(), "demo")
		require.EqualError(t, err, "E-CLI-004: not signed in to the platform at "+p.URL+"\n"+
			"  fix: run aicoded login --org <your organisation>\n  docs: https://aicoded.dev/docs/errors/E-CLI-004", "mode %o", mode)
		assert.NoFileExists(t, filepath.Join(dir, "credentials.lock"))
	}

	require.NoError(t, os.Chmod(dir, 0o700))
	require.NoError(t, platform.Save(p.URL, platform.Credentials{Org: "acme"}))
	_, err := c.Migrations(t.Context(), "demo")
	assert.Equal(t, "E-CLI-004", errs.Code(err), "signed out")

	file := filepath.Join(dir, "credentials.json")
	require.NoError(t, os.Chmod(file, 0o200))
	_, err = c.Migrations(t.Context(), "demo")
	assert.Equal(t, "E-CLI-004", errs.Code(err), "a file that cannot be read")
	require.ErrorContains(t, err, "not signed in to the platform at "+p.URL+": open "+file+": permission denied")
	assert.Zero(t, p.Requests("/v1/apps/migrations"))
}

// A sign-in that others can read is still refused.
func TestMigrationsRefusesASignInOthersCanRead(t *testing.T) {
	p, c := signedIn(t)
	dir, err := os.UserConfigDir()
	require.NoError(t, err)
	require.NoError(t, os.Chmod(filepath.Join(dir, "aicoded", "credentials.json"), 0o644))
	_, err = c.Migrations(t.Context(), "demo")
	assert.Equal(t, "E-CLI-007", errs.Code(err))
	assert.Zero(t, p.Requests("/v1/apps/migrations"))
}

func TestMigrationsRefusesABadList(t *testing.T) {
	p, c := signedIn(t)
	_, err := c.CreatePublish(t.Context(), upload(sha))
	require.NoError(t, err)
	good := step(1, "pub_aaaaaaaaaaaaaaaaaaaaaaaaaa", 1, "CREATE TABLE a (x INT)")
	sum := sha256.Sum256([]byte("CREATE TABLE b (x INT)"))
	for name, s := range map[string]migrate.Step{
		"seq from 2":    {Seq: 2, Publish: good.Publish, Index: 1, SHA256: good.SHA256, Statement: good.Statement},
		"draft":         {Seq: 1, Publish: "draft", Index: 1, SHA256: good.SHA256, Statement: good.Statement},
		"index 0":       {Seq: 1, Publish: good.Publish, Index: 0, SHA256: good.SHA256, Statement: good.Statement},
		"other hash":    {Seq: 1, Publish: good.Publish, Index: 1, SHA256: hex.EncodeToString(sum[:]), Statement: good.Statement},
		"no statement":  {Seq: 1, Publish: good.Publish, Index: 1, SHA256: strings.Repeat("0", 64)},
		"upper-case id": {Seq: 1, Publish: strings.ToUpper(good.Publish), Index: 1, SHA256: good.SHA256, Statement: good.Statement},
	} {
		p.SetMigrations("demo", s)
		_, err := c.Migrations(t.Context(), "demo")
		require.EqualError(t, err, "the platform's answer about the migrations of demo is not valid", name)
	}
}

func TestCreatePublishCarriesTheDraft(t *testing.T) {
	p, c := signedIn(t)
	u := upload(sha)
	u.Migration = strings.Repeat("\"\x01", 32<<10)
	_, err := c.CreatePublish(t.Context(), u)
	require.NoError(t, err, "a draft of 64 KiB, whose JSON is larger")
	require.Len(t, p.Uploads(), 1)
	assert.Equal(t, u.Migration, p.Uploads()[0].Migration)
}
