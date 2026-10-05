// Package bad2 reaches the default logger through pointer conversions written with type parameters.
package bad2

import "log/slog"

// logger is slog.Logger under the app's own name.
type logger slog.Logger

// conv converts a pointer to slog.Logger, held as a type parameter, to a pointer to logger.
func conv[P *slog.Logger](p P) *logger { return (*logger)(p) }

// conv2 converts between two pointer types, both held as type parameters.
func conv2[P *slog.Logger, Q *logger](p P) Q { return Q(p) }

// Replace replaces the default logger through conv and conv2.
func Replace(h slog.Handler) {
	*conv(slog.Default()) = logger(*slog.New(h))
	*conv2[*slog.Logger, *logger](slog.Default()) = logger(*slog.New(h))
}
