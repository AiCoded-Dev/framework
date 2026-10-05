package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Names of the hidden inputs every generated form carries.
const (
	CSRFField = "_aicoded_csrf"
	FormField = "_aicoded_form"
)

const (
	csrfLifetime = 24 * time.Hour
	csrfSkew     = time.Minute
	csrfNonce    = 16
	csrfMaxLen   = 128
)

var b64 = base64.RawURLEncoding

// mintCSRF returns b64(nonce) "." issued-at "." b64(mac): a token that only this app's key
// could sign, bound to the viewer's subject.
func mintCSRF(key []byte, app, subject string, now time.Time) string {
	nonce := make([]byte, csrfNonce)
	_, _ = rand.Read(nonce)
	iat := strconv.FormatInt(now.Unix(), 10)
	return b64.EncodeToString(nonce) + "." + iat + "." + b64.EncodeToString(csrfMAC(key, app, subject, nonce, iat))
}

// verifyCSRF reports whether token was minted with key for app and subject and is still valid.
// It never accepts a token for an empty subject.
func verifyCSRF(key []byte, app, subject, token string, now time.Time) bool {
	if subject == "" || len(token) > csrfMaxLen {
		return false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	nonce, err := b64.DecodeString(parts[0])
	if err != nil || len(nonce) != csrfNonce {
		return false
	}
	mac, err := b64.DecodeString(parts[2])
	if err != nil || !hmac.Equal(mac, csrfMAC(key, app, subject, nonce, parts[1])) {
		return false
	}
	iat, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return false
	}
	age := now.Unix() - iat
	return age >= -int64(csrfSkew/time.Second) && age <= int64(csrfLifetime/time.Second)
}

func csrfMAC(key []byte, app, subject string, nonce []byte, iat string) []byte {
	m := hmac.New(sha256.New, key)
	_, _ = fmt.Fprintf(m, "aicoded-csrf.v1|%d:%s|%d:%s|%d:%s|%s", len(app), app, len(subject), subject, len(nonce), nonce, iat)
	return m.Sum(nil)
}
