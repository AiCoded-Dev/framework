package secrets_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"

	"aicoded.dev/framework/secrets"
)

// Reveal a secret only where it is used, here in a page's Data to sign a link so that the app can
// later tell that it made the link itself.
func ExampleGet() {
	data := func(ctx context.Context) error {
		key, err := secrets.Get(ctx, "link_key") // secrets: [link_key] in the permission list
		if err != nil {
			return err
		}
		mac := hmac.New(sha256.New, []byte(key.Reveal()))
		mac.Write([]byte("/reports/2026-09"))
		fmt.Println("/reports/2026-09?sig=" + hex.EncodeToString(mac.Sum(nil)))
		return nil
	}
	_ = data
}

// A Value shows as [REDACTED] wherever it is printed, logged or encoded, so a slip never shows
// the secret.
func ExampleValue() {
	var key secrets.Value // as secrets.Get returns it
	fmt.Println(key)
	data, err := json.Marshal(struct{ Key secrets.Value }{key})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(data))
	// Output:
	// [REDACTED]
	// {"Key":"[REDACTED]"}
}
