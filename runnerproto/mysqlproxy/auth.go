package mysqlproxy

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // the MySQL protocol requires SHA-1
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
)

// nonceLen is the length of the server's nonce that the scrambles use.
const nonceLen = 20

var errNonce = errors.New("mysqlproxy: the server's nonce is too short")

// scramble returns the login token of password for plugin and the server's nonce, or nil for
// an empty password. Plugins other than mysql_native_password and caching_sha2_password are
// refused.
func scramble(plugin, password string, nonce []byte) ([]byte, error) {
	if plugin != "mysql_native_password" && plugin != "caching_sha2_password" {
		return nil, fmt.Errorf("mysqlproxy: the server asks for the auth plugin %q", plugin)
	}
	if password == "" {
		return nil, nil
	}
	if len(nonce) < nonceLen {
		return nil, errNonce
	}
	nonce = nonce[:nonceLen]
	if plugin == "mysql_native_password" {
		h1 := sha1.Sum([]byte(password)) //nolint:gosec // the MySQL protocol requires SHA-1
		h2 := sha1.Sum(h1[:])            //nolint:gosec // the MySQL protocol requires SHA-1
		h := sha1.New()                  //nolint:gosec // the MySQL protocol requires SHA-1
		h.Write(nonce)
		h.Write(h2[:])
		out := h.Sum(nil)
		for i := range out {
			out[i] ^= h1[i]
		}
		return out, nil
	}
	m1 := sha256.Sum256([]byte(password))
	m2 := sha256.Sum256(m1[:])
	h := sha256.New()
	h.Write(m2[:])
	h.Write(nonce)
	out := h.Sum(nil)
	for i := range out {
		out[i] ^= m1[i]
	}
	return out, nil
}

// encryptPassword encrypts password for caching_sha2_password's full authentication over a
// connection MySQL does not count as secure.
func encryptPassword(pemKey []byte, password string, nonce []byte) ([]byte, error) {
	if len(nonce) < nonceLen {
		return nil, errNonce
	}
	nonce = nonce[:nonceLen]
	block, _ := pem.Decode(pemKey)
	if block == nil {
		return nil, errors.New("mysqlproxy: the server's public key is not PEM")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	pub, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("mysqlproxy: the server's public key is not RSA")
	}
	plain := append([]byte(password), 0)
	for i := range plain {
		plain[i] ^= nonce[i%len(nonce)]
	}
	return rsa.EncryptOAEP(sha1.New(), rand.Reader, pub, plain, nil) //nolint:gosec // the MySQL protocol requires SHA-1
}
