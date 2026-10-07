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

// localTLDs are the last labels of names that lead to this computer or a private network.
var localTLDs = []string{"localhost", "local", "internal", "arpa"}

const maxTTLDays = 3650

const (
	fixClass    = "use the app type's name: lower-case letters, digits and dashes, such as internal-tool"
	fixOwner    = "write the group that owns the app as group:<name>, such as group:hr-leads: lower-case letters, digits, dots, dashes and underscores"
	fixAudience = "list people of the company as group:<name> or everyone under internal, and outside people as idp:<name> or magic-link under external, each once"
	fixEgress   = "write the host alone, lower-case, such as api.partner.example: no scheme, port, path, wildcard or IP address, nothing local, each once"
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

func (c *checker) checkEgress(hosts []string, n *yaml.Node) {
	seen := map[string]bool{}
	for i, h := range hosts {
		at := line(item(n, i))
		switch {
		case slices.Contains(localTLDs, h[strings.LastIndexByte(h, '.')+1:]):
			c.add(at, "E-MAN-018", fmt.Sprintf("egress entry %q is a local name, which an app may not reach", h), fixEgress)
		case len(h) > 253 || !hostName.MatchString(h):
			c.add(at, "E-MAN-018", fmt.Sprintf("egress entry %q is not a host name", h), fixEgress)
		case seen[h]:
			c.add(at, "E-MAN-018", fmt.Sprintf("egress entry %q is listed twice", h), fixEgress)
		}
		seen[h] = true
	}
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
