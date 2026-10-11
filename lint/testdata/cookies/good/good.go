// Package good sets and reads only __Host- cookies with Path=/, Secure and no Domain.
package good

import (
	"errors"
	"net/http"

	"aicoded.dev/framework/web"
)

// theme names the cookie of the theme.
const theme = "__Host-theme"

// Set sets the theme cookie.
func Set(w web.ResponseWriter, v string) {
	c := &http.Cookie{Name: theme, Value: v, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode}
	w.Header().Add("Set-Cookie", c.String())
	c.Value, c.MaxAge = "", -1
	c.Name, c.Path, c.Secure, c.Domain = "__Host-other", "/", true, ""
	w.Header().Add("Set-Cookie", c.String())
	w.Header().Add("Set-Cookie", (&http.Cookie{Name: theme, Path: "/", Secure: true, MaxAge: -1}).String())
	w.Header().Set("Set-Cookie", "__Host-lang=pt; Path=/; Secure; HttpOnly")
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Add(key(), v)
	all := []http.Cookie{{Name: theme, Path: "/", Secure: true}}
	_ = all
}

// Read returns the theme a viewer chose.
func Read(r *web.Request, name string) string {
	c, err := r.Cookie(theme)
	if err != nil {
		return ""
	}
	_ = r.CookiesNamed(theme)
	_, _ = r.Cookie(name)
	return c.Value
}

func key() string { return errors.New("X-Theme").Error() }
