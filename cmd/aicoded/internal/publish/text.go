package publish

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"aicoded.dev/framework/cmd/aicoded/internal/platform"
	"aicoded.dev/framework/cmd/aicoded/internal/problem"
)

// WriteText writes the publish as aicoded publish and aicoded status print it: what was
// published, each step with its outcome, the problems as aicoded check prints them, each note
// after "note: ", which checks ran, the change record, and a last line with the status, which
// counts the problems of a failed publish.
func WriteText(w io.Writer, p platform.Publish) error {
	var b strings.Builder
	fmt.Fprintf(&b, "publish %s of %s at %s", p.ID, p.App, short(p.SHA))
	if p.Summary != "" {
		fmt.Fprintf(&b, ": %s", p.Summary)
	}
	b.WriteString("\n")
	width := 9
	for _, s := range p.Steps {
		width = max(width, utf8.RuneCountInString(s.Name))
	}
	for _, s := range p.Steps {
		fmt.Fprintf(&b, "  %-*s %s\n", width, s.Name, s.Outcome)
	}
	for _, q := range p.Problems {
		writeProblem(&b, "", p.App, q)
	}
	for _, q := range p.Notes {
		writeProblem(&b, "note: ", p.App, q)
	}
	if p.HistoryRewritten {
		b.WriteString("  history rewritten: the commit does not descend from the one published before it\n")
	}
	switch p.Checks {
	case "":
	case "all-but-l7":
		b.WriteString("  checks: the security checks of layers L1 to L5 ran; simulated attacks (L7) come later\n")
	case "partial":
		b.WriteString("  partial checks: not every security check ran; the steps above say which\n")
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

// writeProblem writes the problem q of app on lines of their own, indented, after prefix.
func writeProblem(b *strings.Builder, prefix, app string, q problem.Problem) {
	q.App = app
	fmt.Fprintf(b, "  %s%s\n", prefix, strings.ReplaceAll(q.String(), "\n", "\n  "))
}
