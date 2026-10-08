# E-GATE-026: go.mod has a godebug line

`go.mod` has a `godebug` line or block, which changes how Go behaves at run time: some of its settings turn off security defaults, such as `tlsrsakex=1`, which brings back TLS key exchange without forward secrecy, or `tls3des=1`, which brings back the weak 3DES ciphers. The delivery pipeline keeps Go's defaults for every app, and refuses the line. The problem is at the line.

```text
module rooms

go 1.25.0

godebug tlsrsakex=1   // wrong: remove it
```

**Fix:** remove the godebug line from go.mod: the platform keeps Go's security defaults.
