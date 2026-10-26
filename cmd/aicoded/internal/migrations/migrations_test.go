package migrations

import (
	"bytes"
	"context"
	"errors"
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
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/migrate"
)

const pub = "pub_aaaaaaaaaaaaaaaaaaaaaaaaaa"

var (
	notes     = manifest.Manifest{App: "notes", Data: []manifest.Data{{Source: "sqldb", Classes: []string{"internal"}}}}
	published = Append(nil, pub, []string{"CREATE TABLE a (x INT)"})
)

// fake is a platform that answers steps, or fails with err.
type fake struct {
	steps []migrate.Step
	err   error
}

func (f fake) Migrations(context.Context, string) ([]migrate.Step, error) { return f.steps, f.err }

// app returns the folder of an app whose draft is text, or which has none when text is "".
func app(t *testing.T, text string) string {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".aicoded"), 0o755))
	if text != "" {
		writeDraft(t, dir, text)
	}
	return dir
}

func writeDraft(t *testing.T, dir, text string) {
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".aicoded", "migration.sql"), []byte(text), 0o644))
}

func exists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

func TestAppend(t *testing.T) {
	steps := Append(published, Draft, []string{"CREATE TABLE b (y INT)", "CREATE INDEX i ON b (y)"})
	require.Len(t, steps, 3)
	assert.Equal(t, migrate.Step{Seq: 3, Publish: "draft", Index: 2, SHA256: hash([]byte("CREATE INDEX i ON b (y)")),
		Statement: "CREATE INDEX i ON b (y)"}, steps[2])
}

func TestSteps(t *testing.T) {
	dir := app(t, "CREATE TABLE b (y INT);\n")
	steps, warning, err := Steps(t.Context(), dir, notes, fake{steps: published})
	require.NoError(t, err)
	assert.Empty(t, warning)
	assert.Equal(t, Append(published, Draft, []string{"CREATE TABLE b (y INT)"}), steps, "the published steps, then the draft's")

	steps, warning, err = Steps(t.Context(), dir, notes, fake{err: errors.New("reach the platform: connection refused")})
	require.NoError(t, err)
	assert.Equal(t, "could not get the migrations notes published from the platform: reach the platform: connection refused; "+
		"the ones cached in .aicoded/migrations.json are applied", warning)
	assert.Equal(t, Append(published, Draft, []string{"CREATE TABLE b (y INT)"}), steps, "from the cache")

	steps, _, err = Steps(t.Context(), dir, notes, nil)
	require.NoError(t, err)
	assert.Len(t, steps, 2, "with no platform, from the cache without a warning")
}

func TestStepsWithoutCache(t *testing.T) {
	dir := app(t, "CREATE TABLE b (y INT);\n")
	steps, warning, err := Steps(t.Context(), dir, notes, fake{err: errors.New("down")})
	require.NoError(t, err)
	assert.Equal(t, "could not get the migrations notes published from the platform: down; only the draft is applied", warning)
	assert.Equal(t, Append(nil, Draft, []string{"CREATE TABLE b (y INT)"}), steps)

	_, _, err = Steps(t.Context(), dir, notes, fake{})
	require.NoError(t, err)
	assert.False(t, exists(dir, Cache), "an app with nothing published gets no cache")
}

func TestStepsWithoutASignIn(t *testing.T) {
	dir := app(t, "CREATE TABLE b (y INT);\n")
	out := fake{err: errs.New("E-CLI-004", "not signed in to the platform at https://a.example", "run aicoded login")}
	steps, warning, err := Steps(t.Context(), dir, notes, out)
	require.NoError(t, err)
	assert.Equal(t, "E-CLI-004: not signed in to the platform at https://a.example; only the draft is applied", warning)
	assert.Equal(t, Append(nil, Draft, []string{"CREATE TABLE b (y INT)"}), steps)

	_, _, err = Steps(t.Context(), dir, notes, fake{steps: published})
	require.NoError(t, err)
	steps, warning, err = Steps(t.Context(), dir, notes, out)
	require.NoError(t, err)
	assert.Equal(t, "E-CLI-004: not signed in to the platform at https://a.example; the migrations cached in .aicoded/migrations.json "+
		"are applied", warning)
	assert.Len(t, steps, 2, "the cached step, then the draft's")
}

func TestStepsRefusesTheDraft(t *testing.T) {
	_, _, err := Steps(t.Context(), app(t, "DROP TABLE a;\n"), notes, fake{})
	require.ErrorContains(t, err, ".aicoded/migration.sql:1: E-MIG-003: ")
}

func TestCacheStaysInTheApp(t *testing.T) {
	dir := app(t, "")
	outside := filepath.Join(t.TempDir(), "migrations.json")
	require.NoError(t, os.WriteFile(outside, []byte(`{"steps": []}`), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, Cache)))
	steps, warning := Published(t.Context(), dir, "notes", fake{err: errors.New("down")})
	assert.Empty(t, steps)
	assert.Contains(t, warning, "the cache .aicoded/migrations.json cannot be read")
}

// The cache holds what the platform answered about the steps, and nothing of the sign-in that
// fetched them.
func TestCacheHoldsNoToken(t *testing.T) {
	testhome.Set(t)
	p := platformtest.New(t)
	p.SignIn(t)
	c := platform.New(p.URL, &bytes.Buffer{})
	sha := strings.Repeat("ab", 20)
	_, err := c.CreatePublish(t.Context(), platform.Upload{App: "notes", SHA: sha, Bundle: []byte("# v2 git bundle\n" + sha + " HEAD\n\nPACK")})
	require.NoError(t, err)
	p.SetMigrations("notes", published...)
	dir := app(t, "")

	steps, warning := Published(t.Context(), dir, "notes", c)
	assert.Empty(t, warning)
	assert.Equal(t, published, steps)
	data, err := os.ReadFile(filepath.Join(dir, Cache))
	require.NoError(t, err)
	cr, err := platform.Load(p.URL)
	require.NoError(t, err)
	assert.NotContains(t, string(data), cr.AccessToken)
	assert.NotContains(t, string(data), cr.RefreshToken)
	assert.NotContains(t, string(data), "Bearer")
}

func TestSettlePassed(t *testing.T) {
	dir := app(t, "CREATE TABLE b (y INT);\n")
	require.NoError(t, MarkSent(dir, pub, []byte("CREATE TABLE b (y INT);\n")))
	assert.Equal(t, pub, Pending(dir))

	note, err := Settle(t.Context(), dir, "notes", "pub_bbbbbbbbbbbbbbbbbbbbbbbbbb", "passed", fake{})
	require.NoError(t, err)
	assert.Empty(t, note, "another publish")
	assert.True(t, exists(dir, ".aicoded/migration.sql"))

	note, err = Settle(t.Context(), dir, "notes", pub, "passed", fake{steps: published})
	require.NoError(t, err)
	assert.Equal(t, "publish "+pub+" passed with the schema change in .aicoded/migration.sql, which is cleared", note)
	assert.False(t, exists(dir, ".aicoded/migration.sql"), "the draft is cleared")
	assert.Empty(t, Pending(dir))
	steps, _ := Published(t.Context(), dir, "notes", nil)
	assert.Equal(t, published, steps, "the cache is refreshed")
}

func TestSettleKeepsTheDraft(t *testing.T) {
	dir := app(t, "CREATE TABLE b (y INT);\n")
	require.NoError(t, MarkSent(dir, pub, []byte("CREATE TABLE b (y INT);\n")))
	note, err := Settle(t.Context(), dir, "notes", pub, "failed", fake{})
	require.NoError(t, err)
	assert.Equal(t, "publish "+pub+" failed, so .aicoded/migration.sql stays as it was", note)
	assert.True(t, exists(dir, ".aicoded/migration.sql"), "a failed publish leaves the draft")
	assert.Empty(t, Pending(dir))

	require.NoError(t, MarkSent(dir, pub, []byte("CREATE TABLE b (y INT);\n")))
	writeDraft(t, dir, "CREATE TABLE b (y INT);\nCREATE TABLE c (z INT);\n")
	note, err = Settle(t.Context(), dir, "notes", pub, "passed", fake{})
	require.NoError(t, err)
	assert.Contains(t, note, "the draft changed since, so it stays")
	assert.True(t, exists(dir, ".aicoded/migration.sql"), "a draft changed since it was sent stays")
}

// Between a publish that passed and Settle, the draft it carried is among the published steps,
// and adds none of its own; a draft changed since still does.
func TestStepsSkipAPublishedDraft(t *testing.T) {
	dir := app(t, "CREATE TABLE a (x INT);\n")
	require.NoError(t, MarkSent(dir, pub, []byte("CREATE TABLE a (x INT);\n")))
	steps, _, err := Steps(t.Context(), dir, notes, fake{})
	require.NoError(t, err)
	assert.Len(t, steps, 1, "the publish has not passed")

	steps, _, err = Steps(t.Context(), dir, notes, fake{steps: published})
	require.NoError(t, err)
	assert.Equal(t, published, steps)

	writeDraft(t, dir, "CREATE TABLE a (x INT);\nCREATE TABLE b (y INT);\n")
	steps, _, err = Steps(t.Context(), dir, notes, fake{steps: published})
	require.NoError(t, err)
	assert.Len(t, steps, 3)
}

func TestPublishedFromTheCache(t *testing.T) {
	dir := app(t, "")
	steps, _ := Published(t.Context(), dir, "notes", fake{steps: published})
	require.Equal(t, published, steps)

	steps, warning := Published(t.Context(), dir, "notes", fake{err: ErrUseCache})
	assert.Equal(t, published, steps)
	assert.Empty(t, warning, "the cache is the answer")

	steps, warning = Published(t.Context(), dir, "notes", fake{})
	assert.Empty(t, steps, "the platform's answer is applied")
	assert.Equal(t, "the platform lists no migrations of notes, but .aicoded/migrations.json holds 1 steps: check that "+
		"AICODED_PLATFORM names the platform you publish to; the platform's answer is applied", warning)
}

func TestPublishedFromAnUnreadableCache(t *testing.T) {
	dir := app(t, "")
	require.NoError(t, os.WriteFile(filepath.Join(dir, Cache), []byte("{"), 0o644))
	for _, p := range []Platform{fake{err: ErrUseCache}, nil} {
		steps, warning := Published(t.Context(), dir, "notes", p)
		assert.Empty(t, steps)
		assert.Equal(t, "the cache .aicoded/migrations.json is not valid, so only the draft is applied", warning)
	}
}

func TestForget(t *testing.T) {
	dir := app(t, draftB)
	require.NoError(t, MarkSent(dir, pub, []byte(draftB)))
	require.NoError(t, Forget(dir, "pub_bbbbbbbbbbbbbbbbbbbbbbbbbb"))
	assert.Equal(t, pub, Pending(dir), "another publish")
	require.NoError(t, Forget(dir, pub))
	assert.Empty(t, Pending(dir))
	assert.True(t, exists(dir, ".aicoded/migration.sql"), "the draft stays")
}

const draftB = "CREATE TABLE b (y INT);\n"
