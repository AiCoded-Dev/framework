package check

import (
	"fmt"
	"io"
	"strings"
)

// OK reports whether no app has a problem.
func (r Report) OK() bool {
	for _, a := range r.Apps {
		if len(a.Problems) > 0 {
			return false
		}
	}
	return true
}

// WriteText writes the report as aicoded check prints it: a block per app, the notes, a line
// that sums it up, and a last line when the tests did not run.
func (r Report) WriteText(w io.Writer) error {
	var b strings.Builder
	problems, failed := 0, 0
	for _, a := range r.Apps {
		if len(a.Problems) == 0 {
			fmt.Fprintf(&b, "ok  %s\n", a.Name)
		} else {
			fmt.Fprintf(&b, "FAIL %s\n", a.Name)
			problems += len(a.Problems)
			failed++
		}
		changed := "wrote"
		if r.Frozen {
			changed = "out of date"
		}
		for _, f := range a.Changed {
			fmt.Fprintf(&b, "  %s %s\n", changed, f)
		}
		for _, warning := range a.Warnings {
			fmt.Fprintf(&b, "  warning: %s\n", warning)
		}
		if a.Surface != nil {
			fmt.Fprintf(&b, "  %s\n", a.Surface)
		}
		for _, p := range a.Problems {
			fmt.Fprintf(&b, "  %s\n", strings.ReplaceAll(p.String(), "\n", "\n  "))
		}
	}
	for _, note := range r.Notes {
		fmt.Fprintln(&b, note)
	}
	if r.OK() {
		b.WriteString("aicoded check: ok\n")
	} else {
		fmt.Fprintf(&b, "aicoded check: %s in %s\n", count(problems, "problem"), count(failed, "app"))
	}
	if r.NoTests {
		b.WriteString("tests did not run (--no-tests)\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// count returns n and the noun, in the plural unless n is 1.
func count(n int, noun string) string {
	if n != 1 {
		noun += "s"
	}
	return fmt.Sprintf("%d %s", n, noun)
}
