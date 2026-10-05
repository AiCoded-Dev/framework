// Package generate turns an app's pages/ folder into Go code, assets and live-page scripts.
package generate

import (
	"cmp"
	"slices"
	"strings"

	"aicoded.dev/framework/internal/errs"
)

// Diagnostics is every coded problem one run found, so all broken templates are reported at once.
type Diagnostics []*errs.Error

func (d Diagnostics) Error() string {
	lines := make([]string, len(d))
	for i, e := range d {
		lines[i] = e.Error()
	}
	return strings.Join(lines, "\n")
}

// Unwrap returns the diagnostics as errors, so errors.As and errs.Code find the first one.
func (d Diagnostics) Unwrap() []error {
	out := make([]error, len(d))
	for i, e := range d {
		out[i] = e
	}
	return out
}

// Err returns d sorted by position, or nil when it is empty.
func (d Diagnostics) Err() error {
	if len(d) == 0 {
		return nil
	}
	slices.SortStableFunc(d, func(a, b *errs.Error) int { return cmp.Compare(a.Pos, b.Pos) })
	return d
}
