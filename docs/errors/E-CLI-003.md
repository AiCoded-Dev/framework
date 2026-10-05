# E-CLI-003: this aicoded knows no framework version

A new app's `go.mod` requires the framework version that `aicoded` itself was built with. A released `aicoded` knows its version. An `aicoded` installed with `make install` from a framework checkout knows that checkout instead, and its apps use a `replace` line to it. This `aicoded` was built some other way, for example with `go build` in a checkout, so it knows neither.

**Fix:** install a released `aicoded`, or run `make install` in the framework checkout.
