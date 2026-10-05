package errs

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestError(t *testing.T) {
	err := At("pages/index.html:3", "E-GEN-001", "route has no access rule", `add <ssr:access role="..."/>`)
	assert.Equal(t, "pages/index.html:3: E-GEN-001: route has no access rule\n"+
		"  fix: add <ssr:access role=\"...\"/>\n"+
		"  docs: https://aicoded.dev/docs/errors/E-GEN-001", err.Error())
	assert.Equal(t, "E-RUN-001: runner missing\n  fix: start it\n  docs: https://aicoded.dev/docs/errors/E-RUN-001",
		New("E-RUN-001", "runner missing", "start it").Error())
}

func TestCode(t *testing.T) {
	assert.Equal(t, "E-RUN-001", Code(fmt.Errorf("wrapped: %w", New("E-RUN-001", "m", "f"))))
	assert.Empty(t, Code(errors.New("plain")))
}
