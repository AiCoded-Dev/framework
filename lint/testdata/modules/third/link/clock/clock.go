// Package clock reads the runtime's clock.
package clock

import _ "unsafe"

//go:linkname nanotime runtime.nanotime
func nanotime() int64

// Now returns the runtime's clock.
func Now() int64 { return nanotime() }
