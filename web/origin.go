package web

import (
	"net/http"
	"net/url"
	"strings"
)

// crossSite reports whether a request comes from another site. The browser says so in
// Sec-Fetch-Site, or names another origin in Origin. strict also refuses the opaque origin
// "null", for requests no form token follows. A form post with Origin "null" relies on its
// form token, which is bound to the viewer's subject; this check is defence in depth.
func crossSite(r *http.Request, strict bool) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return false
	case "same-site", "cross-site":
		return true
	}
	origin := r.Header.Get("Origin")
	switch origin {
	case "":
		return false
	case "null":
		return strict
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return true
	}
	scheme := strings.ToLower(u.Scheme)
	return !strings.EqualFold(stripDefaultPort(u.Host, scheme), stripDefaultPort(r.Host, scheme))
}

func stripDefaultPort(host, scheme string) string {
	switch scheme {
	case "https":
		return strings.TrimSuffix(host, ":443")
	case "http":
		return strings.TrimSuffix(host, ":80")
	}
	return host
}
