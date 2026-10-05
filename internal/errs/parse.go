package errs

import (
	"cmp"
	"regexp"
	"strings"
)

var codeName = regexp.MustCompile(`^E-[A-Z]+-[0-9]{3}$`)

// Parse reads back the text an Error without a position prints, such as a runner's refusal
// that crossed a socket: the code, the message and the optional fix and docs lines. A missing
// fix reads as "read the docs page below". It returns false when msg does not start with a code.
func Parse(msg string) (*Error, bool) {
	code, rest, _ := strings.Cut(msg, ": ")
	if !codeName.MatchString(code) {
		return nil, false
	}
	rest, _, _ = strings.Cut(rest, "\n  docs: ")
	text, fix, _ := strings.Cut(rest, "\n  fix: ")
	return New(code, text, cmp.Or(fix, "read the docs page below")), true
}
