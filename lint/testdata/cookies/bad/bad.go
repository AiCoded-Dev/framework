// Package bad sets and reads cookies that app code may not.
package bad

import (
	"net/http"

	"aicoded.dev/framework/web"
)

const theme = "__Host-theme"

type wrap struct{ http.Cookie }

type own http.Cookie

// Set sets cookies that are not __Host- cookies with Path=/, Secure and without Domain.
func Set(w web.ResponseWriter, name, v string) {
	h := w.Header()
	h.Add("Set-Cookie", (&http.Cookie{Name: "sid", Value: v, Path: "/", Secure: true}).String())
	h.Add("Set-Cookie", (&http.Cookie{Name: theme, Value: v, Path: "/"}).String())
	h.Add("Set-Cookie", (&http.Cookie{Name: theme, Path: "/", Secure: true, Domain: "acme.example"}).String())
	secure := true
	h.Add("Set-Cookie", (&http.Cookie{Name: name, Path: "/", Secure: secure}).String())
	h.Add("Set-Cookie", (&http.Cookie{Value: v, Path: "/", Secure: true}).String())
	_ = []*http.Cookie{{Name: "sid", Path: "/", Secure: true}}
	c := &http.Cookie{Name: theme, Path: "/", Secure: true}
	c.Name = name
	c.Secure = false
	c.Domain = "acme.example"
	c.Name += "x"
	_ = &c.Secure
	_ = new(http.Cookie)
	var zero http.Cookie
	_ = zero
	h.Set("set-cookie", "sid="+v)
	h.Add("Set-Cookie", "__Host-a=b; Path=/")
	h.Add("Set-Cookie", "__Host-a=b; Path=/; Secure; Domain=acme.example")
	h["Set-Cookie"] = []string{c.String()}
	_ = http.Header{"Set-Cookie": {c.String()}}
	http.Header.Add(h, "Set-Cookie", v)
	http.SetCookie(nil, c)
	wc := wrap{Cookie: http.Cookie{Name: theme, Path: "/", Secure: true}}
	wc.Secure = false
	_ = own{Name: "sid", Path: "/", Secure: true}
}

// SetPath sets cookies without Path=/, which browsers drop for a __Host- cookie.
func SetPath(w web.ResponseWriter) {
	h := w.Header()
	h.Add("Set-Cookie", (&http.Cookie{Name: theme, Secure: true}).String())
	h.Add("Set-Cookie", (&http.Cookie{Name: theme, Path: "/rooms", Secure: true}).String())
	h.Add("Set-Cookie", (&http.Cookie{Name: theme, Secure: true, MaxAge: -1}).String())
	c := &http.Cookie{Name: theme, Path: "/", Secure: true}
	c.Path = "/rooms"
	h.Add("Set-Cookie", "__Host-a=b; Secure")
}

// Read reads cookies that are not __Host- cookies, and hands out methods lint cannot follow.
func Read(w web.ResponseWriter, r *web.Request) {
	_, _ = r.Cookie("sid")
	_ = r.CookiesNamed("sid")
	_, _ = (*http.Request).Cookie(r.Request, "session")
	read := r.Cookie
	_, _ = read(theme)
	add := w.Header().Add
	add("Set-Cookie", "sid=1")
	_ = http.Header.Set
}
