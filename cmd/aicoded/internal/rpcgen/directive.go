package rpcgen

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"aicoded.dev/framework/cmd/aicoded/internal/manifest"
)

// directive starts the line that says who may call a function.
const directive = "//ssr:access"

// roleName is the shape of a role, as in <ssr:access>.
var roleName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// isDirective reports whether the comment text is an //ssr:access line.
func isDirective(text string) bool {
	rest, ok := strings.CutPrefix(text, directive)
	return ok && (rest == "" || rest[0] == ' ' || rest[0] == '\t')
}

// isSpaced reports whether the comment text is an //ssr:access line written with white space
// after the //, which is not read as one.
func isSpaced(text string) bool {
	rest, ok := strings.CutPrefix(text, "//")
	trimmed := strings.TrimLeft(rest, " \t")
	return ok && len(trimmed) < len(rest) && isDirective("//"+trimmed)
}

// parseAccess reads the attributes of an //ssr:access line: caller=<app>[,<app>…], then
// role=<rule> and apps=true, at least one of the two, in any order.
func parseAccess(text string) (Access, error) {
	var a Access
	seen := map[string]bool{}
	for _, attr := range strings.Fields(strings.TrimPrefix(text, directive)) {
		key, value, ok := strings.Cut(attr, "=")
		if !ok {
			return Access{}, fmt.Errorf("%q is not an attribute; write key=value", attr)
		}
		if seen[key] {
			return Access{}, fmt.Errorf("%s= is given twice", key)
		}
		seen[key] = true
		switch key {
		case "caller":
			for app := range strings.SplitSeq(value, ",") {
				if !manifest.ValidApp(app) {
					return Access{}, fmt.Errorf("caller %q is not an app name", app)
				}
				a.Callers = append(a.Callers, app)
			}
		case "role":
			rule, err := roleRule(value)
			if err != nil {
				return Access{}, err
			}
			a.Require = []string{rule}
		case "apps":
			if value != "true" {
				return Access{}, fmt.Errorf("apps=%s; write apps=true or leave it out", value)
			}
			a.Apps = true
		default:
			return Access{}, fmt.Errorf("%s= is not an attribute of //ssr:access", key)
		}
	}
	if len(a.Callers) == 0 {
		return Access{}, errors.New("caller= is missing")
	}
	if a.Require == nil && !a.Apps {
		return Access{}, errors.New("role= and apps=true are both missing, so nobody may call it")
	}
	slices.Sort(a.Callers)
	a.Callers = slices.Compact(a.Callers)
	return a, nil
}

// roleRule returns the rule of role=value as the access section writes it: the roles sorted and
// joined by "|", or "*".
func roleRule(value string) (string, error) {
	roles := strings.Split(value, "|")
	for _, r := range roles {
		if r != "*" && !roleName.MatchString(r) {
			return "", fmt.Errorf("role %q is not a lower-case name or *", r)
		}
	}
	if len(roles) > 1 && slices.Contains(roles, "*") {
		return "", fmt.Errorf(`role=%s: "*" admits every viewer, so it stands alone`, value)
	}
	slices.Sort(roles)
	return strings.Join(slices.Compact(roles), "|"), nil
}
