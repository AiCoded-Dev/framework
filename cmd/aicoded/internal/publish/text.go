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
// after "note: ", which checks ran, the change record, the decision on releasing it, an approver's
// decline, and a last line with the status, which counts the problems of a failed publish.
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
	case "all":
		b.WriteString("  checks: the security checks of layers L1 to L5, L8 and L9, and the simulated attacks (L7), ran\n")
	case "all-but-l7":
		b.WriteString("  checks: the security checks of layers L1 to L5, L8 and L9 ran; simulated attacks (L7) come later\n")
	case "partial":
		b.WriteString("  partial checks: not every security check ran; the steps above say which\n")
	default:
		fmt.Fprintf(&b, "  checks: %s\n", p.Checks)
	}
	if p.Record > 0 {
		fmt.Fprintf(&b, "  change record %d\n", p.Record)
	}
	if p.Release != nil {
		writeRelease(&b, p.ID, *p.Release, p.Declined != nil)
	}
	if d := p.Declined; d != nil {
		fmt.Fprintf(&b, "  Declined by %s%s.\n  Publish again and ask for its release to get a new approval card.\n"+
			"  If it changed the schema, put the changes you still want back in .aicoded/migration.sql: they left the app's migrations.\n", d.By, reason(strings.ReplaceAll(d.Reason, "\n", " ")))
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

// writeRelease writes the decision r on releasing the publish id, in the words people see: the
// tier with its app type or its reasons, what became of the release, which an approver must
// approve unless it is declined, how to release a publish held for preview, the older publish it
// replaced, and the address of a live release.
func writeRelease(b *strings.Builder, id string, r platform.Release, declined bool) {
	if r.Tier == "R3" {
		b.WriteString("  Not allowed" + reason(r.Reason) + ".\n")
		return
	}
	switch r.State {
	case "failed":
		b.WriteString("  Release failed" + reason(r.Reason) + ".\n")
		return
	case "refused":
		b.WriteString("  Not released" + reason(r.Reason) + ".\n")
		return
	case "superseded":
		b.WriteString("  Not released: a newer publish of the app came after it.\n")
		return
	}
	switch r.Tier {
	case "R1":
		by := ""
		if appType, _, _ := strings.Cut(r.AppType, "@"); appType != "" {
			by = " by app type " + appType
		}
		switch r.State {
		case "held":
			fmt.Fprintf(b, "  Pre-approved%s.\n", by)
		case "replaced":
			fmt.Fprintf(b, "  Pre-approved%s.\n  Released: %s.\n", by, progress(r))
		default:
			fmt.Fprintf(b, "  Pre-approved%s: %s.\n", by, progress(r))
		}
	case "R2":
		b.WriteString("  Needs approval:\n")
		for _, q := range r.Reasons {
			line := strings.ReplaceAll(q.Message, "\n", " ")
			if q.Code != "" {
				line = q.Code + ": " + line
			}
			fmt.Fprintf(b, "    %s\n", line)
		}
		switch {
		case r.Live || r.State == "released" || r.State == "replaced":
			fmt.Fprintf(b, "  Released: %s.\n", progress(r))
		case r.State == "held":
			b.WriteString("  It will need approval once you ask for its release.\n")
		case !declined:
			b.WriteString("  An approver must approve it before it goes live.\n")
		}
	default:
		fmt.Fprintf(b, "  release: %s, %s\n", r.Tier, r.State)
	}
	if r.State == "held" {
		fmt.Fprintf(b, "  Held for preview, not live: its owner asks for its release with aicoded release %s.\n", id)
	}
	if r.Replaced != "" {
		fmt.Fprintf(b, "  %s\n", Replaced(r))
	}
	if r.Live && r.URL != "" {
		fmt.Fprintf(b, "  Live at %s\n", r.URL)
	}
}

// Replaced says which older publish the release r replaced, and what became of it, or "" when it
// replaced none. While r is held, it says how to get a new approval card or go live.
func Replaced(r platform.Release) string {
	if r.Replaced == "" {
		return ""
	}
	if r.ReplacedState == "waiting" {
		s := "This publish replaced " + r.Replaced + ", which was waiting for approval; its approval card is gone."
		if r.State == "held" {
			s += " Ask for this one's release to get a new card."
		}
		return s
	}
	s := "This publish replaced " + r.Replaced + ", which was going live; it will not go live."
	if r.State == "held" {
		s += " Ask for this one's release instead."
	}
	return s
}

// progress says how far the release of r got.
func progress(r platform.Release) string {
	switch {
	case r.Live:
		return "live"
	case r.State == "released":
		return "being released"
	case r.State == "replaced":
		return "replaced by a newer release"
	case r.State == "due":
		return "going live"
	case r.State != "":
		return r.State
	default:
		return "not released yet"
	}
}

// reason returns ": " and the text of a reason without its last full stop, or "" for none.
func reason(text string) string {
	if text == "" {
		return ""
	}
	return ": " + strings.TrimSuffix(text, ".")
}
