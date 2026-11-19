package describe

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// WriteText writes the summary as aicoded describe prints it: a compact block per app, with no
// line for what an app does not have.
func (s Summary) WriteText(w io.Writer) error {
	var b strings.Builder
	for _, a := range s.Apps {
		fmt.Fprintf(&b, "app %s (%s)\n", a.Name, a.Dir)
		value(&b, "class", a.Class)
		value(&b, "owner", a.Owner)
		if a.Audience != nil {
			line(&b, "audience", a.Audience.Internal)
			line(&b, "external audience", a.Audience.External)
		}
		for _, p := range a.Pages {
			fmt.Fprintf(&b, "  page %s  require %s", p.Path, join(p.Require))
			if p.Guard {
				b.WriteString("  guard")
			}
			if p.Shared {
				b.WriteString("  shared")
			}
			if len(p.Calls) > 0 {
				fmt.Fprintf(&b, "  calls %s", join(p.Calls))
			}
			b.WriteString("\n")
		}
		for _, sv := range a.Serves {
			fmt.Fprintf(&b, "  serves %s  callers %s", sv.Name, join(sv.Callers))
			if len(sv.Require) > 0 {
				fmt.Fprintf(&b, "  require %s", join(sv.Require))
			}
			if sv.Apps {
				b.WriteString("  apps")
			}
			b.WriteString("\n")
		}
		for _, app := range slices.Sorted(maps.Keys(a.Calls)) {
			fmt.Fprintf(&b, "  calls %s: %s\n", app, join(a.Calls[app]))
		}
		if a.SQLDB {
			b.WriteString("  database\n")
		}
		line(&b, "files", a.Stores)
		line(&b, "connectors", a.Connectors)
		if a.Email != nil {
			fmt.Fprintf(&b, "  email from %s", a.Email.From)
			if len(a.Email.ToDomains) > 0 {
				fmt.Fprintf(&b, " to %s", join(a.Email.ToDomains))
			}
			b.WriteString("\n")
		}
		line(&b, "egress", a.Egress)
		for _, k := range a.Bindings {
			fmt.Fprintf(&b, "  binding %s  secret %s  header %s", k.Host, k.Secret, k.Header)
			if k.Prefix != "" {
				fmt.Fprintf(&b, "  prefix %q", k.Prefix)
			}
			b.WriteString("\n")
		}
		for _, j := range a.Schedule {
			fmt.Fprintf(&b, "  schedule %s  cron %s\n", j.Name, j.Cron)
		}
		line(&b, "settings", a.Settings)
		line(&b, "secrets", a.Secrets)
		line(&b, "modules", a.Modules)
		value(&b, "size", a.Size)
		value(&b, "resources", a.Resources)
		value(&b, "ttl", a.TTL)
		line(&b, "writes", a.Surface.Writes)
		fmt.Fprintf(&b, "  %s\n", a.Surface)
		for _, warning := range a.Warnings {
			fmt.Fprintf(&b, "  warning: %s\n", warning)
		}
		for _, f := range a.Findings {
			fmt.Fprintf(&b, "  finding: %s\n", strings.ReplaceAll(f.String(), "\n", "\n  "))
		}
		writeLocal(&b, a.Local)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// writeLocal writes the local setup l: the workspace in dev.yaml or the note that there is none,
// or the problem that stops aicoded dev from reading it; then the MySQL server, the personas and
// whether each value is set.
func writeLocal(b *strings.Builder, l Local) {
	switch {
	case l.Problem != nil:
		fmt.Fprintf(b, "  local problem: %s\n", strings.ReplaceAll(l.Problem.String(), "\n", "\n  "))
		return
	case l.Workspace != "":
		fmt.Fprintf(b, "  local workspace %s\n", printable(l.Workspace))
	case l.Note != "":
		fmt.Fprintf(b, "  local note: %s\n", printable(l.Note))
	}
	if l.MySQL != nil {
		fmt.Fprintf(b, "  local mysql %s\n", set(*l.MySQL))
	}
	for _, p := range l.Personas {
		fmt.Fprintf(b, "  local persona %s", printable(p.Name))
		if len(p.Roles) > 0 {
			fmt.Fprintf(b, "  roles %s", printable(join(p.Roles)))
		}
		if len(p.Groups) > 0 {
			fmt.Fprintf(b, "  groups %s", printable(join(p.Groups)))
		}
		b.WriteString("\n")
	}
	if len(l.Personas) == 0 && l.Workspace != "" {
		b.WriteString("  local personas none: aicoded dev signs in as all-roles or no-roles\n")
	}
	for _, v := range l.Settings {
		fmt.Fprintf(b, "  local setting %s %s\n", v.Name, set(v.Set))
	}
	for _, v := range l.Secrets {
		fmt.Fprintf(b, "  local secret %s %s\n", v.Name, set(v.Set))
	}
}

// printable returns s, from dev.yaml, with each control or format character, and each byte that
// is not UTF-8, as "?", so that it cannot change the terminal or start a line of its own.
func printable(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == utf8.RuneError {
			return '?'
		}
		return r
	}, s)
}

func set(ok bool) string {
	if ok {
		return "set"
	}
	return "not set"
}

// line writes "  <label> <items>" when there are items.
func line(b *strings.Builder, label string, items []string) {
	if len(items) > 0 {
		fmt.Fprintf(b, "  %s %s\n", label, join(items))
	}
}

// value writes "  <label> <v>" when v is not empty.
func value(b *strings.Builder, label, v string) {
	if v != "" {
		fmt.Fprintf(b, "  %s %s\n", label, v)
	}
}

func join(items []string) string {
	return strings.Join(items, ", ")
}
