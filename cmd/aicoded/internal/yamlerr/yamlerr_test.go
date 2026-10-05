package yamlerr_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"

	"aicoded.dev/framework/cmd/aicoded/internal/yamlerr"
)

func TestLine(t *testing.T) {
	var v struct{ A []string }
	line, ok := yamlerr.Line(yaml.Unmarshal([]byte("b: 1\na: x\n"), &v))
	assert.True(t, ok)
	assert.Equal(t, 2, line)

	_, ok = yamlerr.Line(yaml.Unmarshal([]byte("a: b: c\n"), &v))
	assert.False(t, ok)
}
