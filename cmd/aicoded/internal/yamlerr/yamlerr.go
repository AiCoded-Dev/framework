// Package yamlerr reads source positions out of yaml.v3 errors.
package yamlerr

import (
	"regexp"
	"strconv"
)

var lineRe = regexp.MustCompile(`line (\d+)`)

// Line returns the first line number a yaml.v3 error mentions.
func Line(err error) (int, bool) {
	m := lineRe.FindStringSubmatch(err.Error())
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}
