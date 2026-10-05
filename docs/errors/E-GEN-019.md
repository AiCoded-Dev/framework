# E-GEN-019: `ssr:bind` on a non-scalar variable

An input's `ssr:bind` names a variable that is not a string, bool or number. An input sends its
value as text, which the server reads only into `string`, `bool`, `int`, `int8`–`int64`,
`uint`, `uint8`–`uint64`, `byte`, `rune`, `float32` or `float64`. A named type such as
`type UserID string`, a slice, a map or a struct cannot be bound. A page script can still write
such a client-writable variable as JSON with `ssr.set()`.

**Fix:** bind a string, bool or number, or write the value from index.ts with `ssr.set()`
