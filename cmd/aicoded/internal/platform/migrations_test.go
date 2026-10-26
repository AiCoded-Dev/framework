package platform_test

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
