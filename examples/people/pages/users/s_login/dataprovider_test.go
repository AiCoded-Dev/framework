package s_login

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitials(t *testing.T) {
	assert.Equal(t, "AM", initials("Alice Moreau"))
	assert.Equal(t, "JS", initials("  jane  van der smith "))
	assert.Equal(t, "C", initials("Cher"))
	assert.Equal(t, "ÉZ", initials("émile zola"))
	assert.Equal(t, "?", initials(""))
}
