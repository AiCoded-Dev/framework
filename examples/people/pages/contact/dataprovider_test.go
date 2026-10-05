package contact

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContactKey(t *testing.T) {
	key := contactKey("alice", "Billing", "Please call me back.")
	assert.Regexp(t, `^contact-[0-9a-f]{32}$`, key)
	assert.Equal(t, key, contactKey("alice", "Billing", "Please call me back."), "a message sent twice has one key")
	assert.NotEqual(t, key, contactKey("bob", "Billing", "Please call me back."))
	assert.NotEqual(t, key, contactKey("alice", "Feedback", "Please call me back."))
	assert.NotEqual(t, key, contactKey("alice", "Billing", "Please call me."))
}

func TestPlainAddress(t *testing.T) {
	assert.True(t, plainAddress("carol@example.com"))
	for _, s := range []string{
		"carol",
		"Carol <carol@example.com>",
		"<carol@example.com>",
		"carol@example.com (Carol)",
		`"carol"@example.com`,
		"carol@example.com\r\nBcc: eve@evil.example",
	} {
		assert.False(t, plainAddress(s), s)
	}
}
