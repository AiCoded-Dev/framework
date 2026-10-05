// Package bad3 converts through type parameters whose type sets hold pointers or cannot be listed.
package bad3

import (
	"fmt"
	"log/slog"
)

// logger is slog.Logger under the app's own name.
type logger slog.Logger

// other is slog.Logger under a second name.
type other slog.Logger

// conv3 converts to *logger from a type parameter whose type set holds two pointer types.
func conv3[P interface{ *slog.Logger | *logger }](p P) *logger { return (*logger)(p) }

// conv5 converts between two type parameters that each have no core type.
func conv5[P interface{ *slog.Logger | *logger }, Q interface{ *logger | *other }](p P) Q {
	return Q(p)
}

// set writes through a pointer whose core type is a pointer to a type parameter.
func set[T any, PT interface{ *T }](p PT, v T) { *p = v }

// Replace replaces the default logger through conv3, conv5 and set.
func Replace(h slog.Handler) {
	*conv3(slog.Default()) = logger(*slog.New(h))
	*conv5[*slog.Logger, *other](slog.Default()) = other(*slog.New(h))
	set[slog.Logger](slog.Default(), *slog.New(h))
}

// number lists number types, none of them a pointer.
type number interface{ ~int | ~float64 }

// Sum converts values of a type parameter whose type set holds no pointer, which stays allowed.
func Sum[N number](xs []N) int {
	s := 0
	for _, x := range xs {
		s += int(x)
	}
	return s
}

// Show converts a value of a type parameter whose type set cannot be listed to an interface,
// which stays allowed.
func Show[T fmt.Stringer](v T) fmt.Stringer {
	return fmt.Stringer(v)
}

// Any converts a value of any type parameter to any, which stays allowed.
func Any[T any](v T) any {
	return any(v)
}
