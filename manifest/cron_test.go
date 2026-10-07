package manifest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCron(t *testing.T) {
	for _, expr := range []string{"0 6 * * *", "*/15 * * * *", "0 9-17 * * 1-5", "30 2 1,15 * *", "0 0 * 12 0", "0  6 * * *", "0-30/10 0 31 1-12/12 6"} {
		assert.Empty(t, cronProblem(expr), expr)
	}
	const fields = "is not five fields separated by spaces: minute, hour, day, month and weekday"
	for expr, want := range map[string]string{
		"0 6 * *":                      fields,
		"0 6 * * * *":                  fields,
		"@daily":                       fields,
		" 0 6 * * *":                   fields,
		"0\t6 * * *":                   fields,
		"60 * * * *":                   "has minute 60, outside 0-59",
		"* 24 * * *":                   "has hour 24, outside 0-23",
		"* * 0 * *":                    "has day 0, outside 1-31",
		"* * * 13 *":                   "has month 13, outside 1-12",
		"* * * * 7":                    "has weekday 7, outside 0-6",
		"* * * * 1-7":                  "has weekday 7, outside 0-6",
		"5-1 * * * *":                  "has minute range 5-1, which runs backwards",
		"*/0 * * * *":                  "has minute step 0, outside 1-59",
		"* * * * */7":                  "has weekday step 7, outside 1-6",
		"0 6 * * MON":                  `has weekday "MON", which is not *, a number, a range or a step`,
		"0 6 ? * *":                    `has day "?", which is not *, a number, a range or a step`,
		"0 6 L * *":                    `has day "L", which is not *, a number, a range or a step`,
		"0 6 1,,2 * *":                 `has day "1,,2", which is not *, a number, a range or a step`,
		"5/10 * * * *":                 `has minute "5/10", which is not *, a number, a range or a step`,
		"+5 * * * *":                   `has minute "+5", which is not *, a number, a range or a step`,
		"0 99999999999999999999 * * *": "has hour 99999999999999999999, outside 0-23",
	} {
		assert.Equal(t, want, cronProblem(expr), expr)
	}
}
