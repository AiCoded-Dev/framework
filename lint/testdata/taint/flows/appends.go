// Package flows: the Append functions of the standard library, which write into the slice they
// get.
package flows

import (
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"

	"aicoded.dev/framework/web"
)

// Appends carries the URL parameter login into the memory of a slice through each Append
// function, and records all of that memory.
func Appends(r *web.Request) {
	login := r.URLParam("login")
	mem := make([]byte, 0, 64)
	_ = fmt.Append(mem, login)
	slog.Info("fmt.Append", "v", mem[:cap(mem)]) // leak: a URL parameter reaches slog.Info
	mem = make([]byte, 0, 64)
	_ = fmt.Appendf(mem, "%s", login)
	slog.Info("fmt.Appendf", "v", mem[:cap(mem)]) // leak: a URL parameter reaches slog.Info
	mem = make([]byte, 0, 64)
	_ = fmt.Appendln(mem, login)
	slog.Info("fmt.Appendln", "v", mem[:cap(mem)]) // leak: a URL parameter reaches slog.Info
	mem = make([]byte, 0, 64)
	_ = strconv.AppendQuote(mem, login)
	slog.Info("strconv.AppendQuote", "v", mem[:cap(mem)]) // leak: a URL parameter reaches slog.Info
	mem = make([]byte, 0, 64)
	_ = hex.AppendEncode(mem, []byte(login))
	slog.Info("hex.AppendEncode", "v", mem[:cap(mem)]) // leak: a URL parameter reaches slog.Info
	mem = make([]byte, 0, 64)
	_, _ = binary.Append(mem, binary.BigEndian, []byte(login))
	slog.Info("binary.Append", "v", mem[:cap(mem)]) // leak: a URL parameter reaches slog.Info
	mem = make([]byte, 0, 64)
	_ = utf8.AppendRune(mem, []rune(login)[0])
	slog.Info("utf8.AppendRune", "v", mem[:cap(mem)]) // leak: a URL parameter reaches slog.Info
	mem = make([]byte, 0, 64)
	_ = base64.StdEncoding.AppendEncode(mem, []byte(login))
	slog.Info("base64 AppendEncode", "v", mem[:cap(mem)]) // leak: a URL parameter reaches slog.Info
	mem = make([]byte, 0, 64)
	_, _ = base32.StdEncoding.AppendDecode(mem, []byte(login))
	slog.Info("base32 AppendDecode", "v", mem[:cap(mem)]) // leak: a URL parameter reaches slog.Info
	mem = make([]byte, 0, 64)
	_ = time.Now().AppendFormat(mem, login)
	slog.Info("time AppendFormat", "v", mem[:cap(mem)]) // leak: a URL parameter reaches slog.Info
	mem = make([]byte, 0, 64)
	if u, err := url.Parse(login); err == nil {
		_, _ = u.AppendBinary(mem)
	}
	slog.Info("url AppendBinary", "v", mem[:cap(mem)]) // leak: a URL parameter reaches slog.Info
	names := make([]string, 0, 4)
	_ = slices.AppendSeq(names, slices.Values([]string{login}))
	slog.Info("slices.AppendSeq", "v", names[:cap(names)]) // leak: a URL parameter reaches slog.Info
}
