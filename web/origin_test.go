package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCrossSite(t *testing.T) {
	cases := []struct {
		fetchSite, origin string
		cross, strict     bool
	}{
		{"same-origin", "", false, false},
		{"none", "", false, false},
		{"same-site", "http://other.localhost:8080", true, true},
		{"cross-site", "https://evil.example", true, true},
		{"", "", false, false},
		{"", "http://site.localhost:8080", false, false},
		{"", "HTTP://SITE.localhost:8080", false, false},
		{"", "http://other.localhost:8080", true, true},
		{"", "http://site.localhost:9090", true, true},
		{"", "null", false, true},
		{"", "::bad", true, true},
	}
	for _, c := range cases {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://site.localhost:8080/", nil)
		if c.fetchSite != "" {
			r.Header.Set("Sec-Fetch-Site", c.fetchSite)
		}
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		assert.Equal(t, c.cross, crossSite(r, false), "%+v", c)
		assert.Equal(t, c.strict, crossSite(r, true), "strict %+v", c)
	}

	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://site.example/", nil)
	r.Header.Set("Origin", "https://site.example:443")
	assert.False(t, crossSite(r, true), "default port")
}
