// Package viewerauth admits only requests that carry a valid viewer token from the runner.
package viewerauth

import (
	"crypto/ed25519"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"aicoded.dev/framework/internal/identity"
	"aicoded.dev/framework/runnerproto/viewer"
)

var errWrongPath = errors.New("viewer token: a call token outside the call endpoint, or a viewer token on it")

// Middleware verifies the token of every request for app and puts what it names in the request
// context. A request to callPath must carry a call token, which names the calling app; any
// other request must carry a viewer token, which names none. Requests without a fitting, valid
// token get 401 and never reach next.
//
// A viewer token and a call token made for a viewer put the viewer and the raw token in the
// context; a call token also puts the calling app. A token for a call an app makes as itself
// puts only the calling app.
func Middleware(keys []ed25519.PublicKey, app, callPath string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get(viewer.Header)
		c, err := viewer.Verify(keys, token, app, time.Now())
		if call := c.Caller != ""; err == nil && call != (r.URL.Path == callPath) {
			err = errWrongPath
		}
		if err != nil {
			slog.WarnContext(r.Context(), "request refused", "reason", err.Error())
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		r.Header.Del(viewer.Header)
		ctx := r.Context()
		if c.Caller != "" {
			ctx = identity.WithCaller(ctx, c.Caller)
		}
		if !c.App {
			ctx = identity.With(ctx, identity.Identity{Subject: c.Subject, Name: c.Name, Groups: c.Groups, Roles: c.Roles})
			ctx = identity.WithToken(ctx, token)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
