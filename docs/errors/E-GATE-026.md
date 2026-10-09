# E-GATE-026: the app changes Go's security defaults

The app changes how Go behaves at run time, with a `godebug` line or block in `go.mod`, with a `//go:debug` comment in a Go file, or with a `go` line in `go.mod` older than the Go release that the delivery pipeline builds with. Some of these settings turn off security defaults, such as `tlsrsakex=1`, which brings back TLS key exchange without forward secrecy, or `tls3des=1`, which brings back the weak 3DES ciphers. The `go` line selects defaults too: Go keeps the behaviour of the release that the line names, so an old line selects older, weaker defaults. The delivery pipeline keeps Go's defaults for every app, and refuses each such setting at its line: in `go.mod` every `godebug` setting, and a `go` line older than the release of its Go, such as `go 1.25.0` when it builds with Go 1.26, which the message names as `1.26.0`; and in the commit's Go files, generated files and tests among them, every line comment that starts with `//go:debug`.

```text
module rooms

go 1.26.0

godebug tlsrsakex=1   // wrong: remove it
```

```go
//go:debug tlsrsakex=1   // wrong: remove it

package main
```

```sh
go mod edit -go=1.26.0   # the release the message names
go mod tidy
```

**Fix:** the one for the case the message names:

- a `godebug` setting in `go.mod`: remove the godebug line from go.mod: the platform keeps Go's security defaults;
- in a Go file: remove the //go:debug line: the platform keeps Go's security defaults;
- an old `go` line in `go.mod`: raise the go line in go.mod to the release the message names: `go mod edit -go=<release>`, then go mod tidy, commit, and publish again.
