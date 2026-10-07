// Package platform signs a builder in to the aicoded platform and keeps the sign-in: aicoded
// login, logout and whoami, and the token that other commands send to the platform. It signs in
// with OAuth 2.1, in the browser through a loopback redirect (RFC 8252) or with a code entered
// on any device (RFC 8628).
package platform

import (
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"aicoded.dev/framework/internal/errs"
)

// DefaultAddress is the platform's address when AICODED_PLATFORM is not set.
const DefaultAddress = "https://api.aicoded.cloud"

// Address returns the platform's address from AICODED_PLATFORM, DefaultAddress when it is not
// set, written scheme://host, without the port of an https address when it is 443. It must be
// https, or http to 127.0.0.1 with a port, with no user, path, query or fragment; anything else
// is E-CLI-005, which does not quote the value, since it could hold a password.
func Address() (string, error) {
	s := os.Getenv("AICODED_PLATFORM")
	if s == "" {
		return DefaultAddress, nil
	}
	u, err := url.Parse(s)
	if err == nil && u.Opaque == "" && u.User == nil && u.Host != "" && u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" &&
		(u.Path == "" || u.Path == "/") && (u.Port() != "" || !strings.HasSuffix(u.Host, ":")) {
		switch {
		case u.Scheme == "https" && u.Port() == "443":
			return u.Scheme + "://" + strings.ToLower(strings.TrimSuffix(u.Host, ":443")), nil
		case u.Scheme == "https" && (u.Port() == "" || validPort(u.Port())):
			return u.Scheme + "://" + strings.ToLower(u.Host), nil
		case u.Scheme == "http" && u.Hostname() == "127.0.0.1" && validPort(u.Port()):
			return u.Scheme + "://" + u.Host, nil
		}
	}
	return "", errs.New("E-CLI-005", "AICODED_PLATFORM is not a valid address of the platform",
		"set AICODED_PLATFORM to an https address with no path, such as "+DefaultAddress+", or to http://127.0.0.1:<port> for a platform on this computer, or unset it")
}

func validPort(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n > 0 && n < 65536 && strconv.Itoa(n) == s
}

// orgName is an organisation's name, as the platform allows it.
var orgName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,38}$`)

// ValidOrg reports whether s can name an organisation: 2 to 39 lowercase letters, digits and
// dashes, starting with a letter or digit.
func ValidOrg(s string) bool { return orgName.MatchString(s) }

// clean returns text from the platform fit to print: on one line, without control and format
// characters, which could steer the terminal, and at most limit runes long.
func clean(s string, limit int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == utf8.RuneError {
			return '?'
		}
		return r
	}, s)
	if utf8.RuneCountInString(s) > limit {
		s = string([]rune(s)[:limit]) + "..."
	}
	return s
}

// visible reports whether s is 1 to limit visible ASCII characters, as tokens and codes are.
func visible(s string, limit int) bool {
	if s == "" || len(s) > limit {
		return false
	}
	for i := range len(s) {
		if s[i] < 0x21 || s[i] > 0x7e {
			return false
		}
	}
	return true
}
