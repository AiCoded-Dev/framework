package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"aicoded.dev/framework/cmd/aicoded/internal/platform"
	"aicoded.dev/framework/manifest"
)

// runPreview runs aicoded preview <id>, or aicoded preview end [dir], with the arguments after
// the command.
func runPreview(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "end" {
		return runPreviewEnd(args[1:], stdout, stderr)
	}
	fs := newFlags("preview", stderr)
	rest, code, ok := parseArgs(fs, args, 1, 1)
	if !ok {
		return code
	}
	id := rest[0]
	if !platform.PublishID.MatchString(id) {
		fmt.Fprintf(stderr, "aicoded preview: %.40q is not the id of a publish, which is pub_ and 26 lowercase letters and digits\n\n", id)
		fs.Usage()
		return exitUsage
	}
	c, err := platformClient(stdout)
	if err != nil {
		return fail(stderr, err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pv, err := c.PreviewStart(ctx, id)
	if err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintf(stdout, "Preview of %s at %s until %s.\n", pv.App, pv.URL, pv.Expires.Format(time.RFC3339))
	if pv.Reset {
		fmt.Fprintln(stdout, "Its data was reset: the preview had run schema changes that this publish does not continue.")
	}
	fmt.Fprintln(stdout, "Invite up to five people in the console.")
	return exitOK
}

// runPreviewEnd runs aicoded preview end with the arguments after it.
func runPreviewEnd(args []string, stdout, stderr io.Writer) int {
	rest, code, ok := parseArgs(newFlags("preview end", stderr), args, 0, 1)
	if !ok {
		return code
	}
	m, err := manifest.Load(filepath.Join(dirArg(rest), manifest.FileName))
	if err != nil {
		return fail(stderr, err)
	}
	c, err := platformClient(stdout)
	if err != nil {
		return fail(stderr, err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := c.PreviewEnd(ctx, m.App); err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintf(stdout, "Preview of %s ended; its data is being deleted.\n", m.App)
	return exitOK
}
