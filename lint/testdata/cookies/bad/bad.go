// Package bad sets cookies that app code may not set.
package bad

import (
	"net/http"

	"aicoded.dev/framework/web"
)

const theme = "__Host-theme"

type wrap struct{ http.Cookie }

type own http.Cookie

// Set sets cookies that are not __Host- cookies with Secure and without Domain.
func Set(w web.ResponseWriter, name, v string) {
	h := w.Header()
	h.Add("Set-Cookie", (&http.Cookie{Name: "sid", Value: v, Secure: true}).String())
	h.Add("Set-Cookie", (&http.Cookie{Name: theme, Value: v}).String())
	h.Add("Set-Cookie", (&http.Cookie{Name: theme, Secure: true, Domain: "acme.example"}).String())
	secure := true
	h.Add("Set-Cookie", (&http.Cookie{Name: name, Secure: secure}).String())
	h.Add("Set-Cookie", (&http.Cookie{Value: v, Secure: true}).String())
	_ = []*http.Cookie{{Name: "sid", Secure: true}}
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
	wc := wrap{Cookie: http.Cookie{Name: theme, Secure: true}}
	wc.Secure = false
	_ = own{Name: "sid", Secure: true}
}
