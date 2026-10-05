package run

import (
	"crypto/tls"
	"go/build"
	"go/importer"
	"go/token"
	"log/syslog"
)

// Secure connects to addr over TLS.
func Secure(addr string) (*tls.Conn, error) { return tls.Dial("tcp", addr, nil) }

// Log connects to the syslog server at addr.
func Log(addr string) (*syslog.Writer, error) {
	return syslog.Dial("tcp", addr, syslog.LOG_INFO, "run")
}

// Find runs the go command of root to find the package at path.
func Find(root, path string) (*build.Package, error) {
	c := build.Default
	c.GOROOT = root
	return c.Import(path, ".", 0)
}

// Source returns an importer that runs the cgo tool for packages that use cgo.
func Source() any { return importer.ForCompiler(token.NewFileSet(), "source", nil) }
