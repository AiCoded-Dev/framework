# E-RPC-003: the rpc package or one of its functions has the wrong shape

Other apps call a function of `rpc/` with one input value and get back one output value or an
error, so every exported function has the shape `func Name(ctx context.Context, in In) (Out,
error)`. The parameter names are free and `context` may be imported under any name; `In` and
`Out` are exported struct types declared in the package, without type parameters, and the
function has no type parameters and no variadic parameter. The package in `rpc/` is named
`rpc`, and none of its types takes the name of a type Go declares itself, such as `string`.
`aicoded generate` adds `rpc/server_gen.go` to the package, so it also refuses a method named
`aicodedAppend` or `aicodedDecode` on any type, and a package-level name that file uses:
`Server`, `context`, `frameworkrpc`, `protowire` or `wire`.

**Fix:** write `func Name(ctx context.Context, in NameIn) (NameOut, error)` with exported struct types of package rpc, and rename anything that takes a name Go or the generator uses.
