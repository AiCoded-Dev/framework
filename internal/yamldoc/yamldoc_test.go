package yamldoc_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/yamldoc"
)

func TestLine(t *testing.T) {
	var v struct{ A []string }
	line, ok := yamldoc.Line(yaml.Unmarshal([]byte("b: 1\na: x\n"), &v))
	assert.True(t, ok)
	assert.Equal(t, 2, line)

	_, ok = yamldoc.Line(yaml.Unmarshal([]byte("a: b: c\n"), &v))
	assert.False(t, ok)
}

func TestDocument(t *testing.T) {
	doc, err := yamldoc.Document("aicoded.yaml", nil)
	require.NoError(t, err)
	assert.Zero(t, doc.Kind, "no document")

	doc, err = yamldoc.Document("aicoded.yaml", []byte("app: rooms\n"))
	require.NoError(t, err)
	assert.Equal(t, yaml.MappingNode, doc.Content[0].Kind)

	for src, pos := range map[string]string{"a: [\n": "aicoded.yaml:1", "a: 1\n---\nb: 2\n": "aicoded.yaml:2"} {
		_, err = yamldoc.Document("aicoded.yaml", []byte(src))
		var e *errs.Error
		require.ErrorAs(t, err, &e, src)
		assert.Equal(t, pos+" E-MAN-003", e.Pos+" "+e.Code, src)
	}
}
