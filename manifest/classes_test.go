package manifest_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/manifest"
)

const everyone = "audience: {internal: [everyone]}\n"

// problem returns the one problem Parse reports in src.
func problem(t *testing.T, src string) *errs.Error {
	_, err := manifest.Parse(manifest.FileName, []byte(src))
	var e *errs.Error
	require.ErrorAs(t, err, &e, src)
	return e
}

func TestDataClasses(t *testing.T) {
	m := load(t, "app: a1\n"+everyone+"data:\n  - source: sqldb\n    classes: [internal, personal]\n")
	assert.Equal(t, []string{"internal", "personal"}, m.Data[0].Classes)

	e := problem(t, "app: a1\n"+everyone+"data:\n  - source: sqldb\n    classes: [secret]\n")
	assert.Equal(t, []string{"aicoded.yaml:4", "E-MAN-008", `data class "secret" of sqldb is not public, internal, confidential or personal`,
		"use public, internal, confidential or personal"}, []string{e.Pos, e.Code, e.Msg, e.Fix})

	e = problem(t, "app: a1\n"+everyone+"data:\n  - source: sqldb\n    classes: [internal, hr]\n")
	assert.Equal(t, []string{"aicoded.yaml:4", "E-MAN-024", `data class "hr" of sqldb is not allowed: apps may not hold health, HR or card data`,
		"remove hr from the classes of sqldb, and the data from the app"}, []string{e.Pos, e.Code, e.Msg, e.Fix})
}

func TestNotAllowed(t *testing.T) {
	for src, want := range map[string]string{
		"app: a1\n":            "aicoded.yaml:1",
		"app: a1\naudience:\n": "aicoded.yaml:2",
		"app: a1\naudience: {internal: [], external: []}\n":                                  "aicoded.yaml:2",
		"app: a1\n" + everyone + "data: [{source: sqldb, classes: [health]}]\n":              "aicoded.yaml:3",
		"app: a1\n" + everyone + "data:\n  - source: filestore:cards\n    classes: [card]\n": "aicoded.yaml:4",
	} {
		e := problem(t, src)
		assert.Equal(t, want+" E-MAN-024", e.Pos+" "+e.Code, src)
	}
	e := problem(t, "app: a1\n")
	assert.Equal(t, "the audience lists no one, and an app that no one signs in to is not allowed", e.Msg)
	assert.Equal(t, "list who uses the app under audience, such as audience: {internal: [everyone]} or {internal: [group:<name>]}", e.Fix)

	for _, src := range []string{"audience: {internal: [group:ops]}\n", "audience: {external: [magic-link]}\n"} {
		_, err := manifest.Parse(manifest.FileName, []byte("app: a1\n"+src))
		assert.NoError(t, err, src)
	}
}

func TestClassRank(t *testing.T) {
	for i, class := range []string{"public", "internal", "confidential", "personal"} {
		rank, ok := manifest.ClassRank(class)
		assert.True(t, ok, class)
		assert.Equal(t, i, rank, class)
	}
	for _, class := range []string{"health", "hr", "card", "secret", "", "Internal"} {
		_, ok := manifest.ClassRank(class)
		assert.False(t, ok, class)
	}
}

func TestAdmits(t *testing.T) {
	assert.True(t, manifest.Audience{Internal: []string{"everyone"}}.Admits(nil))
	assert.True(t, manifest.Audience{Internal: []string{"group:hr", "group:ops"}}.Admits([]string{"sales", "ops"}))
	assert.False(t, manifest.Audience{Internal: []string{"group:hr"}}.Admits([]string{"ops", "group:hr", "everyone"}))
	assert.False(t, manifest.Audience{External: []string{"magic-link", "idp:partners"}}.Admits([]string{"magic-link", "partners"}))
	assert.False(t, manifest.Audience{Internal: []string{"group:"}}.Admits([]string{""}))
}

func TestValidHost(t *testing.T) {
	for _, h := range []string{"api.partner.example", "a.io"} {
		assert.True(t, manifest.ValidHost(h), h)
	}
	for _, h := range []string{"", "localhost", "printer.local", "metadata.google.internal", "API.partner.example", "10.0.0.1", "a.example:443", "*.a.example"} {
		assert.False(t, manifest.ValidHost(h), h)
	}
}
