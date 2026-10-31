package manifest

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

var (
	groupName = regexp.MustCompile(`^[a-z0-9]([a-z0-9._-]{0,62}[a-z0-9])?$`)
	hostName  = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
	days      = regexp.MustCompile(`^[1-9][0-9]{0,3}d$`)
)

// platformDomains are the domains of the platform, which no app may reach.
var platformDomains = []string{"aicoded.cloud", "aicoded.dev"}

// reservedHeaders are the headers, in lower case, that may not carry a key: the hop-by-hop ones,
// Host, Content-Length, Content-Type and Cookie.
var reservedHeaders = []string{"connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "proxy-connection", "te",
	"trailer", "transfer-encoding", "upgrade", "host", "content-length", "content-type", "cookie"}

// maxBindingLen is the most bytes of a bound header's name and of its prefix.
const maxBindingLen = 64

// localTLDs are the last labels of names that lead to this computer or a private network.
var localTLDs = []string{"localhost", "local", "internal", "arpa"}

const maxTTLDays = 3650

const (
	fixClass    = "use the app type's name: lower-case letters, digits and dashes, such as internal-tool"
	fixOwner    = "write the group that owns the app as group:<name>, such as group:hr-leads: lower-case letters, digits, dots, dashes and underscores"
	fixAudience = "list people of the company as group:<name> or everyone under internal, and outside people as idp:<name> or magic-link under external, each once"
	fixEgress   = "write the host alone, lower-case, such as api.partner.example: no scheme, port, path, wildcard or IP address, nothing local, each once"
	fixBinding  = `bind a declared secret to a header such as Authorization, with an optional prefix such as "Bearer "`
	fixSchedule = `write each job as {cron: "<minute> <hour> <day> <month> <weekday>", job: <name>}, such as {cron: "0 6 * * *", job: daily-summary} for 06:00 UTC every day, each job once`
	fixTTL      = "write a number of days from 1 to 3650 followed by d, such as 180d"
)

func (c *checker) checkClass(class string, n *yaml.Node) {
	if class != "" && !lowerName.MatchString(class) {
		c.add(line(n), "E-MAN-015", fmt.Sprintf("class %q is not a valid app type name", class), fixClass)
	}
}

func (c *checker) checkOwner(owner string, n *yaml.Node) {
	if owner != "" && !isGroup(owner) {
		c.add(line(n), "E-MAN-016", fmt.Sprintf("owner %q is not group:<name>", owner), fixOwner)
	}
}

func isGroup(ref string) bool {
	name, ok := strings.CutPrefix(ref, "group:")
	return ok && groupName.MatchString(name)
}

func (c *checker) checkAudience(a Audience, n *yaml.Node) {
	if len(a.Internal) == 0 && len(a.External) == 0 {
		c.add(c.keyLine("audience"), "E-MAN-024", "the audience lists no one, and an app that no one signs in to is not allowed",
			"list who uses the app under audience, such as audience: {internal: [everyone]} or {internal: [group:<name>]}")
	}
	c.checkRefs(a.Internal, "internal", "group:<name> or everyone", value(n, "internal"), func(ref string) bool {
		return ref == "everyone" || isGroup(ref)
	})
	c.checkRefs(a.External, "external", "idp:<name> or magic-link", value(n, "external"), func(ref string) bool {
		name, ok := strings.CutPrefix(ref, "idp:")
		return ref == "magic-link" || ok && lowerName.MatchString(name)
	})
}

func (c *checker) checkRefs(refs []string, key, forms string, n *yaml.Node, valid func(string) bool) {
	seen := map[string]bool{}
	for i, ref := range refs {
		switch {
		case !valid(ref):
			c.add(line(item(n, i)), "E-MAN-017", fmt.Sprintf("audience.%s entry %q is not %s", key, ref, forms), fixAudience)
		case seen[ref]:
			c.add(line(item(n, i)), "E-MAN-017", fmt.Sprintf("audience.%s entry %q is listed twice", key, ref), fixAudience)
		}
		seen[ref] = true
	}
}

// ValidHost reports whether host is a host name an app may reach: lower-case, with no scheme,
// port, path, wildcard or IP address, and not a local name, one whose last label is localhost,
// local, internal or arpa.
func ValidHost(host string) bool {
	return !localHost(host) && len(host) <= 253 && hostName.MatchString(host)
}

func localHost(host string) bool {
	return slices.Contains(localTLDs, host[strings.LastIndexByte(host, '.')+1:])
}

// PlatformHost reports whether host, in any case and with or without a dot at the end, is a
// host of the platform: one of its domains, such as aicoded.cloud, or a subdomain of one.
func PlatformHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	return slices.ContainsFunc(platformDomains, func(d string) bool {
		return host == d || strings.HasSuffix(host, "."+d)
	})
}

func (c *checker) checkEgress(entries []Egress, secrets []string, n *yaml.Node) {
	seen := map[string]bool{}
	for i, e := range entries {
		entry := item(n, i)
		at := line(entry)
		h := e.Host
		switch {
		case h == "":
			c.add(at, "E-MAN-018", "egress entry has no host", fixEgress)
		case PlatformHost(h):
			c.add(at, "E-MAN-018", fmt.Sprintf("egress entry %q is a host of the platform, which an app may not reach", h), fixEgress)
		case localHost(h):
			c.add(at, "E-MAN-018", fmt.Sprintf("egress entry %q is a local name, which an app may not reach", h), fixEgress)
		case !ValidHost(h):
			c.add(at, "E-MAN-018", fmt.Sprintf("egress entry %q is not a host name", h), fixEgress)
		case seen[h]:
			c.add(at, "E-MAN-018", fmt.Sprintf("egress entry %q is listed twice", h), fixEgress)
		}
		seen[h] = true
		if field, p := bindingProblem(e, secrets); p != "" {
			if v := value(entry, field); v != nil {
				at = v.Line
			}
			c.add(at, "E-MAN-025", fmt.Sprintf("egress entry %q %s", h, p), fixBinding)
		}
	}
}

// bindingProblem returns what is wrong with the key binding of e, and the field it is in, empty
// for the entry as a whole; or "" when nothing is.
func bindingProblem(e Egress, secrets []string) (field, problem string) {
	switch {
	case !e.Bound() && (e.Header != "" || e.Prefix != ""):
		return "", "has a header or a prefix but no secret"
	case !e.Bound():
		return "", ""
	case !slices.Contains(secrets, e.Secret):
		return "secret", fmt.Sprintf("binds the secret %q, which secrets does not declare", e.Secret)
	case e.Header == "":
		return "", fmt.Sprintf("binds the secret %q to no header", e.Secret)
	case len(e.Header) > maxBindingLen || !token(e.Header):
		return "header", fmt.Sprintf("has the header %q, which is not a header name of at most %d characters", e.Header, maxBindingLen)
	case slices.Contains(reservedHeaders, strings.ToLower(e.Header)):
		return "header", fmt.Sprintf("has the header %q, which may not carry a key", e.Header)
	case len(e.Prefix) > maxBindingLen || strings.ContainsFunc(e.Prefix, func(r rune) bool { return r < ' ' || r > '~' }):
		return "prefix", fmt.Sprintf("has a prefix that is not at most %d printable ASCII characters", maxBindingLen)
	}
	return "", ""
}

// tchars are the characters of an HTTP token.
const tchars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!#$%&'*+-.^_`|~"

// token reports whether s is an HTTP token, as header names are.
func token(s string) bool {
	return s != "" && strings.Trim(s, tchars) == ""
}

func (c *checker) checkSchedule(jobs []Job, n *yaml.Node) {
	seen := map[string]bool{}
	for i, j := range jobs {
		entry := item(n, i)
		what := fmt.Sprintf("schedule job %q", j.Name)
		switch {
		case j.Name == "":
			what = "schedule entry"
			msg := what + " has no job"
			if j.Cron != "" {
				msg = fmt.Sprintf("schedule entry with cron %q has no job", j.Cron)
			}
			c.add(line(entry), "E-MAN-019", msg, fixSchedule)
		case !lowerName.MatchString(j.Name):
			c.add(line(value(entry, "job")), "E-MAN-019", what+" is not a valid name", fixSchedule)
		case seen[j.Name]:
			c.add(line(value(entry, "job")), "E-MAN-019", what+" is listed twice", fixSchedule)
		}
		seen[j.Name] = true
		if j.Cron == "" {
			c.add(line(entry), "E-MAN-019", what+" has no cron", fixSchedule)
		} else if p := cronProblem(j.Cron); p != "" {
			c.add(line(value(entry, "cron")), "E-MAN-019", fmt.Sprintf("%s: cron %q %s", what, j.Cron, p), fixSchedule)
		}
	}
}

// checkChoice checks that the value of key, when stated, is one of choices.
func (c *checker) checkChoice(v, key string, n *yaml.Node, code string, choices ...string) {
	if v == "" || slices.Contains(choices, v) {
		return
	}
	list := strings.Join(choices[:len(choices)-1], ", ") + " or " + choices[len(choices)-1]
	c.add(line(n), code, fmt.Sprintf("%s %q is not %s", key, v, list), "use "+list)
}

func (c *checker) checkTTL(ttl string, n *yaml.Node) {
	if ttl == "" {
		return
	}
	if !days.MatchString(ttl) {
		c.add(line(n), "E-MAN-022", fmt.Sprintf("ttl %q is not a number of days", ttl), fixTTL)
		return
	}
	if d, _ := strconv.Atoi(strings.TrimSuffix(ttl, "d")); d > maxTTLDays {
		c.add(line(n), "E-MAN-022", fmt.Sprintf("ttl %q is more than %d days", ttl, maxTTLDays), fixTTL)
	}
}
