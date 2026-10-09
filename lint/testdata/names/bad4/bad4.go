// Package bad4 converts and writes through aliases of type parameters.
package bad4

import "log/slog"

// logger is slog.Logger under the app's own name.
type logger slog.Logger

// other is slog.Logger under a second name.
type other slog.Logger

// Ptr is an alias of a pointer to its type argument.
type Ptr[T any] = *T

// convAlias converts to Q written as its alias B.
func convAlias[P interface{ *slog.Logger | *logger }, Q interface{ *logger | *other }](p P) Q {
	type B = Q
	return B(p)
}

// convAliasFrom converts from P held as its alias B.
func convAliasFrom[P interface{ *slog.Logger | *logger }](p P) *logger {
	type B = P
	var b B = p
	return (*logger)(b)
}

// setAlias writes through Ptr[T].
func setAlias[T any](p Ptr[T], v T) { *p = v }

// Replace replaces the default logger through the aliases.
func Replace(h slog.Handler) {
	*convAlias[*slog.Logger, *other](slog.Default()) = other(*slog.New(h))
	*convAliasFrom(slog.Default()) = logger(*slog.New(h))
	setAlias[slog.Logger](slog.Default(), *slog.New(h))
}
