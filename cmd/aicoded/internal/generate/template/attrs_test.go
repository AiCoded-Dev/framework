package template

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScanAttrs(t *testing.T) {
	got := scanAttrs([]byte(`<a href="x" data-a='y' b=z c =  "w" d/ =e checked>`))
	assert.Equal(t, []rawAttr{
		{name: "href", quoted: true}, {name: "data-a", quoted: true}, {name: "b"}, {name: "c", quoted: true},
		{name: "d"}, {name: "=e"}, {name: "checked"},
	}, got)
	assert.Equal(t, []rawAttr{{name: "name", quoted: true}}, scanAttrs([]byte(`<ssr:form name="a">`)))
	assert.Empty(t, scanAttrs([]byte(`<br/>`)))
}
