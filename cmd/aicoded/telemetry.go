package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"aicoded.dev/framework/cmd/aicoded/internal/platform"
	"aicoded.dev/framework/manifest"
)

// telemetryFlags are the flags aicoded logs and aicoded traces share.
type telemetryFlags struct {
	app, env, since, until, trace *string
	limit                         *int
	json                          *bool
}

func addTelemetryFlags(fs *flag.FlagSet, what string) telemetryFlags {
	return telemetryFlags{
		app:   fs.String("app", "", "read the app with this `name`, instead of the app in the folder"),
		env:   fs.String("env", "", "the `environment` to read, preview or production; it must be given"),
		since: fs.String("since", "", "read from this `time` on: RFC 3339, such as 2026-10-24T09:00:00Z, or a while ago, such as 30m, 6h or 2d; an hour before --until by default"),
		until: fs.String("until", "", "read up to this `time`, as --since takes it; now by default"),
		trace: fs.String("trace", "", "only the "+what+" of this trace `id`"),
		limit: fs.Int("limit", 0, "the newest `rows` to read, 1 to 1000; 100 by default"),
		json:  fs.Bool("json", false, "print the "+what+" as JSON, newest first"),
	}
}

// query returns the query the flags give, of the app in dir when --app is not given, with times
// relative to now. A missing --env is a usage error, which ok false reports.
func (f telemetryFlags) query(fs *flag.FlagSet, dir string, now time.Time) (q platform.TelemetryQuery, ok bool, err error) {
	if *f.env == "" {
		fmt.Fprintf(fs.Output(), "%s: give --env preview or --env production\n\n", fs.Name())
		fs.Usage()
		return q, false, nil
	}
	q = platform.TelemetryQuery{App: *f.app, Env: *f.env, Trace: *f.trace, Limit: *f.limit}
	if q.App == "" {
		m, err := manifest.Load(filepath.Join(dir, manifest.FileName))
		if err != nil {
			return q, true, err
		}
		q.App = m.App
	}
	if q.Since, err = platform.When("--since", *f.since, now); err != nil {
		return q, true, err
	}
	q.Until, err = platform.When("--until", *f.until, now)
	return q, true, err
}

// runLogs runs aicoded logs with the arguments after the command.
func runLogs(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("logs", stderr)
	f := addTelemetryFlags(fs, "log lines")
	level := fs.String("level", "", "only lines of this `level` or higher: DEBUG, INFO, WARN or ERROR")
	rest, code, ok := parseArgs(fs, args, 0, 1)
	if !ok {
		return code
	}
	q, ok, err := f.query(fs, dirArg(rest), time.Now())
	switch {
	case !ok:
		return exitUsage
	case err != nil:
		return fail(stderr, err)
	}
	q.Level = strings.ToUpper(*level)
	c, err := platformClient(stdout)
	if err != nil {
		return fail(stderr, err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	lines, err := c.Logs(ctx, q)
	if err != nil {
		return fail(stderr, err)
	}
	if *f.json {
		return printJSON(lines, stdout, stderr)
	}
	if len(lines) == 0 {
		fmt.Fprintf(stdout, "No log lines of %s in %s.\n", q.App, q.Env)
		return exitOK
	}
	for _, l := range slices.Backward(lines) {
		line := fmt.Sprintf("%s %-5s %s", stamp(l.Time), l.Level, l.Body)
		if l.TraceID != "" {
			line += "  trace " + l.TraceID
		}
		fmt.Fprintln(stdout, line)
	}
	return exitOK
}

// runTraces runs aicoded traces with the arguments after the command.
func runTraces(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("traces", stderr)
	f := addTelemetryFlags(fs, "spans")
	rest, code, ok := parseArgs(fs, args, 0, 1)
	if !ok {
		return code
	}
	q, ok, err := f.query(fs, dirArg(rest), time.Now())
	switch {
	case !ok:
		return exitUsage
	case err != nil:
		return fail(stderr, err)
	}
	c, err := platformClient(stdout)
	if err != nil {
		return fail(stderr, err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var spans []platform.Span
	if q.Trace != "" {
		spans, err = c.Trace(ctx, q)
	} else {
		spans, err = c.Spans(ctx, q)
	}
	if err != nil {
		return fail(stderr, err)
	}
	if *f.json {
		return printJSON(spans, stdout, stderr)
	}
	if len(spans) == 0 {
		fmt.Fprintf(stdout, "No spans of %s in %s.\n", q.App, q.Env)
		return exitOK
	}
	depth := depths(spans)
	for _, s := range slices.Backward(spans) {
		line := fmt.Sprintf("%s %s%s  %s", stamp(s.Time), strings.Repeat("  ", depth[s.SpanID]), s.Name, time.Duration(s.DurationNS))
		if s.Status == "Error" {
			line += "  failed"
			if s.StatusMessage != "" {
				line += ": " + s.StatusMessage
			}
		}
		if q.Trace == "" {
			line += "  trace " + s.TraceID
		}
		fmt.Fprintln(stdout, line)
	}
	return exitOK
}

// depths returns how deep each span is below the spans of the list that hold it.
func depths(spans []platform.Span) map[string]int {
	parent := map[string]string{}
	for _, s := range spans {
		parent[s.SpanID] = s.ParentSpanID
	}
	out := map[string]int{}
	for _, s := range spans {
		n := 0
		for p := parent[s.SpanID]; p != "" && n < len(spans); p = parent[p] {
			if _, ok := parent[p]; !ok {
				break
			}
			n++
		}
		out[s.SpanID] = n
	}
	return out
}

// stamp writes t as the logs show it: in UTC, to the millisecond.
func stamp(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

func printJSON(v any, stdout, stderr io.Writer) int {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fail(stderr, err)
	}
	return exitOK
}
