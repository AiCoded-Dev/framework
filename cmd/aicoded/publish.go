package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aicoded.dev/framework/cmd/aicoded/internal/platform"
	"aicoded.dev/framework/cmd/aicoded/internal/publish"
)

// How often aicoded publish asks the platform for the outcome, and for how long.
var (
	pollEvery = 3 * time.Second
	pollFor   = 35 * time.Minute
)

// runPublish runs aicoded publish with the arguments after the command.
func runPublish(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("publish", stderr)
	app := fs.String("app", "", "publish only if the app in the folder has this `name`")
	summary := fs.String("m", "", "the `summary` of the change, for its change record; the commit's subject when left out")
	noWait := fs.Bool("no-wait", false, "print the publish's id and return, without waiting for the outcome")
	rest, code, ok := parseArgs(fs, args, 0, 1)
	if !ok {
		return code
	}
	c, err := platformClient(stdout)
	if err != nil {
		return fail(stderr, err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cm, err := publish.Prepare(ctx, dirArg(rest), publish.Options{App: *app, Summary: *summary})
	if ce := (*publish.CheckError)(nil); errors.As(err, &ce) {
		if err := ce.Report.WriteText(stdout); err != nil {
			return fail(stderr, err)
		}
	}
	if err != nil {
		return fail(stderr, err)
	}
	sent, err := publish.Send(ctx, c, cm)
	if err != nil {
		return fail(stderr, err)
	}
	if sent.CreatedApp {
		fmt.Fprintf(stdout, "Created the app %s, which you own.\n", sent.App)
	}
	fmt.Fprintf(stdout, "Published %s at %s as %s.\n", sent.App, sent.SHA[:7], sent.ID)
	if *noWait {
		fmt.Fprintf(stdout, "aicoded status %s shows its outcome.\n", sent.ID)
		return exitOK
	}
	return wait(ctx, c, sent.ID, stdout, stderr)
}

// wait waits for the outcome of the publish id, prints it and returns the exit code. When it
// stops waiting first, it says that the publish goes on.
func wait(ctx context.Context, c *platform.Client, id string, stdout, stderr io.Writer) int {
	p, err := publish.Wait(ctx, c, id, pollEvery, pollFor, stdout)
	switch {
	case err == nil:
		return report(p, stdout, stderr)
	case errors.Is(err, publish.ErrTimeout):
		fmt.Fprintf(stderr, "aicoded publish: no outcome after %s\n", pollFor)
	case ctx.Err() == nil:
		fmt.Fprintln(stderr, err)
	}
	fmt.Fprintf(stderr, "the publish goes on: aicoded status %s\n", id)
	return exitFail
}

// report prints the publish p and returns the exit code: 1 when it failed, was refused or ended
// in error, and 0 otherwise.
func report(p platform.Publish, stdout, stderr io.Writer) int {
	if err := publish.WriteText(stdout, p); err != nil {
		return fail(stderr, err)
	}
	if err := publish.Outcome(p); err != nil {
		return fail(stderr, err)
	}
	if p.Status == "failed" {
		return exitFail
	}
	return exitOK
}

// runStatus runs aicoded status with the arguments after the command.
func runStatus(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("status", stderr)
	asJSON := fs.Bool("json", false, "print the publish as JSON")
	rest, code, ok := parseArgs(fs, args, 1, 1)
	if !ok {
		return code
	}
	id := rest[0]
	if !platform.PublishID.MatchString(id) {
		fmt.Fprintf(stderr, "aicoded status: %.40q is not the id of a publish, which is pub_ and 26 lowercase letters and digits\n\n", id)
		fs.Usage()
		return exitUsage
	}
	c, err := platformClient(stdout)
	if err != nil {
		return fail(stderr, err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	p, err := c.Publish(ctx, id)
	if err != nil {
		return fail(stderr, err)
	}
	if !*asJSON {
		return report(p, stdout, stderr)
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(p); err != nil {
		return fail(stderr, err)
	}
	if p.Done() && p.Status != "passed" {
		return exitFail
	}
	return exitOK
}
