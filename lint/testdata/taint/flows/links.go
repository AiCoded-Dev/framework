// Package flows: the pairs of values that share memory after a call outside the app.
package flows

import (
	"bufio"
	"bytes"
	"io"
	"log/slog"
	"strings"

	"aicoded.dev/framework/web"
)

// Links carries the URL parameter login through the two ends of a pipe, and through what a
// scanner, a reader and a writer keep.
func Links(r *web.Request) {
	login := r.URLParam("login")
	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write([]byte(login))
		_ = pw.Close()
	}()
	piped, _ := io.ReadAll(pr)
	slog.Info("pipe", "v", piped) // leak: a URL parameter reaches slog.Info
	kept := make([]byte, 64)
	sc := bufio.NewScanner(strings.NewReader(login))
	sc.Buffer(kept, 64)
	sc.Scan()
	slog.Info("scanner buffer", "v", kept[:len(login)]) // leak: a URL parameter reaches slog.Info
	rd := bufio.NewReader(strings.NewReader(""))
	rd.Reset(strings.NewReader(login))
	line, _ := rd.ReadString('\n')
	slog.Info("reader reset", "v", line) // leak: a URL parameter reaches slog.Info
	var sink bytes.Buffer
	wr := bufio.NewWriter(io.Discard)
	wr.Reset(&sink)
	_, _ = wr.WriteString(login)
	_ = wr.Flush()
	slog.Info("writer reset", "v", sink.String()) // leak: a URL parameter reaches slog.Info
}
