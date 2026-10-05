// Package bad4 converts and writes through generic aliases of type parameters.
package bad4

import "log/slog"

// logger is slog.Logger under the app's own name.
type logger slog.Logger

// other is slog.Logger under a second name.
type other slog.Logger

// Box is an alias of its type argument.
type Box[T any] = T

// Ptr is an alias of a pointer to its type argument.
type Ptr[T any] = *T

// convAlias converts to Q written as Box[Q].
func convAlias[P interface{ *slog.Logger | *logger }, Q interface{ *logger | *other }](p P) Q {
	return Box[Q](p)
}

// convAliasFrom converts from P written as Box[P].
func convAliasFrom[P interface{ *slog.Logger | *logger }](p Box[P]) *logger { return (*logger)(p) }

// setAlias writes through Ptr[T].
func setAlias[T any](p Ptr[T], v T) { *p = v }

// Replace replaces the default logger through the aliases.
func Replace(h slog.Handler) {
	*convAlias[*slog.Logger, *other](slog.Default()) = other(*slog.New(h))
	*convAliasFrom(slog.Default()) = logger(*slog.New(h))
	setAlias[slog.Logger](slog.Default(), *slog.New(h))
}
