# E-RPC-004: a field has a type that calls between apps do not carry

The input and output of a function of `rpc/` travel between apps in the protobuf wire format,
with the field numbers pinned in `.aicoded/rpc.json`, so every field of every struct they reach
has one of these types: `bool`, `int32`, `int64`, `uint32`, `uint64`, `float32`, `float64`,
`string`, `[]byte`, `time.Time`, or an exported struct type of package `rpc`; a slice of one of
them; or a pointer to one of them that is not a slice, for a value that may be missing. Structs
may refer to themselves through slices and pointers. `int`, `uint`, the 8- and 16-bit integers,
maps, arrays, interfaces, channels, funcs, types of other packages, slices of pointers,
pointers to slices, slices of slices other than `[][]byte`, and unexported or embedded fields
are refused, at the field.

**Fix:** give the field an allowed type, for example `int64` for `int` or a slice of structs for a map.
