package describe

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
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
	}
	_, err := io.WriteString(w, b.String())
	return err
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
