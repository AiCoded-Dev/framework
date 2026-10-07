package publish

import (
	"fmt"
	"io"
	"strings"

	"aicoded.dev/framework/cmd/aicoded/internal/platform"
)

// WriteText writes the publish as aicoded publish and aicoded status print it: what was
// published, each step with its outcome, the problems as aicoded check prints them, notes, and a
// last line with the status.
func WriteText(w io.Writer, p platform.Publish) error {
	var b strings.Builder
	fmt.Fprintf(&b, "publish %s of %s at %s", p.ID, p.App, short(p.SHA))
	if p.Summary != "" {
		fmt.Fprintf(&b, ": %s", p.Summary)
	}
	b.WriteString("\n")
	for _, s := range p.Steps {
		fmt.Fprintf(&b, "  %-9s %s\n", s.Name, s.Outcome)
	}
	for _, q := range p.Problems {
		q.App = p.App
		fmt.Fprintf(&b, "  %s\n", strings.ReplaceAll(q.String(), "\n", "\n  "))
	}
	if p.HistoryRewritten {
		b.WriteString("  history rewritten: the commit does not descend from the one published before it\n")
	}
	switch p.Checks {
	case "":
	case "partial":
		b.WriteString("  partial checks: only aicoded check and the tests run; the full security checks come later\n")
	default:
		fmt.Fprintf(&b, "  checks: %s\n", p.Checks)
	}
	if p.Record > 0 {
		fmt.Fprintf(&b, "  change record %d\n", p.Record)
	}
	b.WriteString(p.Status)
	switch {
	case p.Status == "failed":
		n, noun := len(p.Problems), "problem"
		if n != 1 {
			noun += "s"
		}
		fmt.Fprintf(&b, ": %d %s", n, noun)
	case p.Reason != "":
		fmt.Fprintf(&b, ": %s", p.Reason)
	}
	b.WriteString("\n")
	_, err := io.WriteString(w, b.String())
	return err
}
