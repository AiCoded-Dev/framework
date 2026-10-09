# E-GATE-026: the app changes Go's security defaults

The app changes how Go behaves at run time, with a `godebug` line or block in `go.mod`, or with a `//go:debug` comment in a Go file. Some of these settings turn off security defaults, such as `tlsrsakex=1`, which brings back TLS key exchange without forward secrecy, or `tls3des=1`, which brings back the weak 3DES ciphers. The delivery pipeline keeps Go's defaults for every app, and refuses each such setting at its line: in `go.mod` every `godebug` setting, and in the commit's Go files, generated files and tests among them, every line comment that starts with `//go:debug`.

```text
module rooms

go 1.25.0

godebug tlsrsakex=1   // wrong: remove it
```

```go
//go:debug tlsrsakex=1   // wrong: remove it

package main
```

**Fix:** the one for the case the message names:

- in `go.mod`: remove the godebug line from go.mod: the platform keeps Go's security defaults;
- in a Go file: remove the //go:debug line: the platform keeps Go's security defaults.
