package mysqlproxy

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testNonce = []byte("0123456789abcdefghij")

func TestNativeScramble(t *testing.T) {
	token, err := scramble("mysql_native_password", "secret", testNonce)
	require.NoError(t, err)
	h1 := sha1.Sum([]byte("secret"))
	stored := sha1.Sum(h1[:])
	h := sha1.New()
	h.Write(testNonce)
	h.Write(stored[:])
	mix := h.Sum(nil)
	for i := range mix {
		mix[i] ^= token[i]
	}
	check := sha1.Sum(mix)
	assert.Equal(t, stored, check, "the server's check of the token passes")
}

func TestSHA2Scramble(t *testing.T) {
	token, err := scramble("caching_sha2_password", "secret", testNonce)
	require.NoError(t, err)
	m1 := sha256.Sum256([]byte("secret"))
	stored := sha256.Sum256(m1[:])
	h := sha256.New()
	h.Write(stored[:])
	h.Write(testNonce)
	mix := h.Sum(nil)
	for i := range mix {
		mix[i] ^= token[i]
	}
	assert.Equal(t, stored, sha256.Sum256(mix), "the server's check of the token passes")

	empty, err := scramble("caching_sha2_password", "", testNonce)
	require.NoError(t, err)
	assert.Nil(t, empty)
	_, err = scramble("sha256_password", "secret", testNonce)
	require.Error(t, err, "unknown plugins are refused")
	_, err = scramble("mysql_clear_password", "secret", testNonce)
	require.Error(t, err, "the password is never sent in clear text on request")
	_, err = scramble("mysql_clear_password", "", testNonce)
	require.Error(t, err, "unknown plugins are refused without a password too")
	_, err = scramble("mysql_native_password", "secret", testNonce[:8])
	assert.Error(t, err, "a short nonce is refused")
}

// rsaKey returns a new RSA key and its public half in PEM, as MySQL sends it.
func rsaKey(t *testing.T) (*rsa.PrivateKey, []byte) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	return key, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

func TestEncryptPassword(t *testing.T) {
	key, pemKey := rsaKey(t)
	enc, err := encryptPassword(pemKey, "secret", testNonce)
	require.NoError(t, err)
	plain, err := rsa.DecryptOAEP(sha1.New(), nil, key, enc, nil)
	require.NoError(t, err)
	for i := range plain {
		plain[i] ^= testNonce[i%len(testNonce)]
	}
	assert.Equal(t, []byte("secret\x00"), plain)
	_, err = encryptPassword([]byte("not pem"), "secret", testNonce)
	require.Error(t, err)
}

func TestEncryptPasswordRefuses(t *testing.T) {
	_, good := rsaKey(t)
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(pub)
	require.NoError(t, err)
	for name, c := range map[string]struct {
		key, nonce []byte
		password   string
	}{
		"not a key": {pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("junk")}), testNonce, "secret"},
		"not RSA":   {pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), testNonce, "secret"},
		"no nonce":  {good, nil, ""},
	} {
		_, err := encryptPassword(c.key, c.password, c.nonce)
		assert.Error(t, err, name)
	}
}
