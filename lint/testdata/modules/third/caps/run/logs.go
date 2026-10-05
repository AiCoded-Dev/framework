package run

import (
	"crypto/rand"
	"hash/crc32"
	"io"
	"log/slog"
	"time"
)

// Quiet replaces the default logger, and changes variables of the standard library.
func Quiet(h slog.Handler, loc *time.Location, r io.Reader) {
	slog.SetDefault(slog.New(h))
	slog.SetLogLoggerLevel(slog.LevelError)
	_ = slog.NewLogLogger(h, slog.LevelInfo)
	time.Local = loc
	*time.Local = *loc
	crc32.IEEETable[0]++
	for _, rand.Reader = range []io.Reader{r} {
	}
	zone := &time.Local
	_ = zone
}

// Keep reads variables of the standard library and changes only its own, which stays allowed.
func Keep(loc *time.Location) *time.Location {
	zone := time.Local
	if loc != nil {
		zone = loc
	}
	return zone
}
