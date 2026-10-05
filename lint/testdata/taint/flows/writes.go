// Package flows: the other functions of the standard library that write into the slice they get.
package flows

import (
	"bytes"
	"crypto/sha256"
	"io"
	"log/slog"
	"regexp"
	"slices"
	"strings"

	"aicoded.dev/framework/web"
)

// Writes carries the URL parameter login into the memory of a slice through each function, and
// records all of that memory.
func Writes(r *web.Request) {
	login := r.URLParam("login")
	mem := make([]byte, 0, 64)
	h := sha256.New()
	_, _ = h.Write([]byte(login))
	_ = h.Sum(mem)
	slog.Info("hash.Hash.Sum", "v", mem[:cap(mem)]) // leak: a URL parameter reaches slog.Info
	names := make([]string, 1, 4)
	_ = slices.Insert(names, 0, login)
	slog.Info("slices.Insert", "v", names[:cap(names)]) // leak: a URL parameter reaches slog.Info
	names = make([]string, 1, 4)
	_ = slices.Replace(names, 0, 1, login)
	slog.Info("slices.Replace", "v", names[:cap(names)]) // leak: a URL parameter reaches slog.Info
	buf := make([]byte, 64)
	var out bytes.Buffer
	_, _ = io.CopyBuffer(&out, strings.NewReader(login), buf)
	slog.Info("io.CopyBuffer", "v", buf) // leak: a URL parameter reaches slog.Info
	mem = make([]byte, 0, 64)
	re := regexp.MustCompile(`(\w+)`)
	_ = re.ExpandString(mem, "$1", login, re.FindStringSubmatchIndex(login))
	slog.Info("regexp ExpandString", "v", mem[:cap(mem)]) // leak: a URL parameter reaches slog.Info
}
