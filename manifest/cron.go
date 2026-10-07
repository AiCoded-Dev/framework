package manifest

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// cronFields are the five fields of a cron expression, with their ranges.
var cronFields = []struct {
	name     string
	min, max int
}{{"minute", 0, 59}, {"hour", 0, 23}, {"day", 1, 31}, {"month", 1, 12}, {"weekday", 0, 6}}

// cronProblem returns what is wrong with the cron expression expr, as words that follow it in a
// message, or "" when it is valid: five fields separated by spaces, each a comma list of *, */n,
// a, a-b or a-b/n in decimal, within the field's range.
func cronProblem(expr string) string {
	fields := strings.FieldsFunc(expr, func(r rune) bool { return r == ' ' })
	if len(fields) != len(cronFields) || expr[0] == ' ' || expr[len(expr)-1] == ' ' {
		return "is not five fields separated by spaces: minute, hour, day, month and weekday"
	}
	for i, f := range fields {
		if p := cronField(f, cronFields[i].name, cronFields[i].min, cronFields[i].max); p != "" {
			return p
		}
	}
	return ""
}

// cronField returns what is wrong with field f of a cron expression, the field called name whose
// values run from lo to hi, or "".
func cronField(f, name string, lo, hi int) string {
	syntax := fmt.Sprintf("has %s %q, which is not *, a number, a range or a step", name, f)
	for item := range strings.SplitSeq(f, ",") {
		span, step, stepped := strings.Cut(item, "/")
		if span != "*" {
			a, b, ranged := strings.Cut(span, "-")
			if stepped && !ranged {
				return syntax
			}
			if !ranged {
				b = a
			}
			first, ok1 := cronNumber(a)
			last, ok2 := cronNumber(b)
			switch {
			case !ok1 || !ok2:
				return syntax
			case first < lo || first > hi:
				return fmt.Sprintf("has %s %s, outside %d-%d", name, a, lo, hi)
			case last < lo || last > hi:
				return fmt.Sprintf("has %s %s, outside %d-%d", name, b, lo, hi)
			case last < first:
				return fmt.Sprintf("has %s range %s, which runs backwards", name, span)
			}
		}
		if stepped {
			n, ok := cronNumber(step)
			if !ok {
				return syntax
			}
			if n < 1 || n > hi {
				return fmt.Sprintf("has %s step %s, outside 1-%d", name, step, hi)
			}
		}
	}
	return ""
}

// cronNumber returns the value of the decimal number s, with math.MaxInt for one too large for
// an int, and whether s is one.
func cronNumber(s string) (int, bool) {
	if s == "" || strings.Trim(s, "0123456789") != "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return math.MaxInt, true
	}
	return n, true
}
