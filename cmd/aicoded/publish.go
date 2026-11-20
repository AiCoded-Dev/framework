package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"aicoded.dev/framework/cmd/aicoded/internal/platform"
	"aicoded.dev/framework/cmd/aicoded/internal/publish"
	"aicoded.dev/framework/cmd/aicoded/internal/workspace"
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
	release := fs.Bool("release", false, "skip the preview: once the publish passed, it goes live by itself or waits for an approver")
	noWait := fs.Bool("no-wait", false, "print the publish's id and return, without waiting for the outcome")
	if previewFlag(args) {
		fmt.Fprint(stderr, "aicoded publish: --preview is gone: every publish is held for its owner's preview now; --release skips it\n\n")
		fs.Usage()
		return exitUsage
	}
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
	note, err := publish.SettlePending(ctx, c, dirArg(rest))
	if err != nil {
		return fail(stderr, err)
	}
	printNote(stdout, note)
	cm, err := publish.Prepare(ctx, dirArg(rest), publish.Options{App: *app, Summary: *summary, Release: *release})
	if ce := (*publish.CheckError)(nil); errors.As(err, &ce) {
		if err := ce.Report.WriteText(stdout); err != nil {
			return fail(stderr, err)
		}
	}
	if err != nil {
		return fail(stderr, err)
	}
	sent, err := publish.Send(ctx, c, cm)
	if sent.ID == "" {
		return fail(stderr, err)
	}
	if sent.CreatedApp {
		fmt.Fprintf(stdout, "Created the app %s, which you own.\n", sent.App)
	}
	fmt.Fprintf(stdout, "Published %s at %s as %s.\n", sent.App, sent.SHA[:7], sent.ID)
	if err != nil {
		return fail(stderr, err)
	}
	if *release {
		fmt.Fprintln(stdout, "It skips the preview: once it passed, it goes live by itself or waits for an approver.")
	}
	if *noWait {
		fmt.Fprintf(stdout, "aicoded status %s shows its outcome.\n", sent.ID)
		if !*release {
			fmt.Fprintf(stdout, "Once it passed, aicoded preview %s starts its preview. It is not live. When the app's owner asks for its release: aicoded release %s.\n",
				sent.ID, sent.ID)
		}
		return exitOK
	}
	p, code := wait(ctx, c, sent.ID, cm.Dir, stdout, stderr)
	if code != exitOK || p.Release == nil || p.Release.State != "held" {
		return code
	}
	startHeld(ctx, c, sent.ID, stdout, stderr)
	return exitOK
}

// previewLimits is the line on what a preview does not do as production does.
const previewLimits = "The preview keeps its first end, reaches no outside service that production does not reach yet, and keeps its mail in the console."

// previewFlag reports whether args hold the flag preview, which older versions took, among their
// flags: those before the first argument that is not a flag, or before --. The flags of publish
// that take a value, -app and -m, may hold it in the next argument.
func previewFlag(args []string) bool {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" || !strings.HasPrefix(a, "-") || a == "-" {
			return false
		}
		name, _, value := strings.Cut(strings.TrimLeft(a, "-"), "=")
		switch {
		case name == "preview":
			return true
		case (name == "app" || name == "m") && !value:
			i++
		}
	}
	return false
}

// startHeld starts the preview of the publish id, held for its owner's preview, and prints it
// with how to release the publish. A preview that does not start leaves the publish held: its
// problem goes to stderr, and what to do next to stdout.
func startHeld(ctx context.Context, c *platform.Client, id string, stdout, stderr io.Writer) {
	pv, err := c.PreviewStart(ctx, id)
	if err != nil {
		fmt.Fprintln(stderr, err)
		fmt.Fprintf(stdout, "The publish passed and is held; it is not live. Fix that and run aicoded preview %s. "+
			"Its owner may also release it without a preview, only at their word: aicoded release %s.\n", id, id)
		return
	}
	printPreview(stdout, pv)
	fmt.Fprintln(stdout, previewLimits)
	fmt.Fprintf(stdout, "It is not live. When the app's owner asks for its release: aicoded release %s.\n", id)
}

// printNote prints note, a line about the draft, unless it is "".
func printNote(stdout io.Writer, note string) {
	if note != "" {
		fmt.Fprintln(stdout, "aicoded: "+note)
	}
}

// wait waits for the outcome of the publish id of the app in dir, prints it, clears the draft
// once the publish passed, and returns the publish and the exit code. When it stops waiting
// first, it says that the publish goes on.
func wait(ctx context.Context, c *platform.Client, id, dir string, stdout, stderr io.Writer) (platform.Publish, int) {
	p, err := publish.Wait(ctx, c, id, pollEvery, pollFor, stdout)
	switch {
	case err == nil:
		code := report(p, stdout, stderr)
		note, err := publish.Settle(ctx, c, dir, p)
		if err != nil {
			fmt.Fprintln(stderr, "aicoded: the draft stays: "+err.Error())
		}
		printNote(stdout, note)
		return p, code
	case errors.Is(err, publish.ErrTimeout):
		fmt.Fprintf(stderr, "aicoded publish: no outcome after %s\n", pollFor)
	case ctx.Err() == nil:
		fmt.Fprintln(stderr, err)
	}
	fmt.Fprintf(stderr, "the publish goes on: aicoded status %s\n", id)
	return platform.Publish{}, exitFail
}

// report prints the publish p and returns the exit code: 1 when it failed, was refused or ended
// in error, or its approval card was declined, and 0 otherwise.
func report(p platform.Publish, stdout, stderr io.Writer) int {
	if err := publish.WriteText(stdout, p); err != nil {
		return fail(stderr, err)
	}
	if err := publish.Outcome(p); err != nil {
		return fail(stderr, err)
	}
	if p.Status == "failed" || p.Declined != nil {
		return exitFail
	}
	return exitOK
}

// runRelease runs aicoded release with the arguments after the command: it asks for the release
// of a publish held for preview, waits until the release goes live, waits for an approver or
// ends otherwise, and prints the publish. It exits with 1 when the release was refused or failed.
func runRelease(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("release", stderr)
	noWait := fs.Bool("no-wait", false, "print the outcome of the request and return, without waiting for the release to go live")
	rest, code, ok := parseArgs(fs, args, 1, 1)
	if !ok {
		return code
	}
	id := rest[0]
	if !platform.PublishID.MatchString(id) {
		fmt.Fprintf(stderr, "aicoded release: %.40q is not the id of a publish, which is pub_ and 26 lowercase letters and digits\n\n", id)
		fs.Usage()
		return exitUsage
	}
	c, err := platformClient(stdout)
	if err != nil {
		return fail(stderr, err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	p, err := c.Release(ctx, id)
	if err != nil {
		return fail(stderr, err)
	}
	if !*noWait && publish.Releasing(p) {
		if p, err = publish.WaitRelease(ctx, c, id, pollEvery, pollFor, stdout); err != nil {
			switch {
			case errors.Is(err, publish.ErrTimeout):
				fmt.Fprintf(stderr, "aicoded release: not live after %s\n", pollFor)
			case ctx.Err() == nil:
				fmt.Fprintln(stderr, err)
			}
			fmt.Fprintf(stderr, "the release goes on: aicoded status %s\n", id)
			return exitFail
		}
	}
	code = report(p, stdout, stderr)
	if r := p.Release; code == exitOK && r != nil && (r.State == "refused" || r.State == "failed") {
		return exitFail
	}
	return code
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
	note, err := settleHere(ctx, c, p)
	if err != nil {
		fmt.Fprintln(stderr, "aicoded: the draft stays: "+err.Error())
	}
	if !*asJSON {
		code := report(p, stdout, stderr)
		printNote(stdout, note)
		return code
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(p); err != nil {
		return fail(stderr, err)
	}
	if p.Done() && p.Status != "passed" || p.Declined != nil {
		return exitFail
	}
	return exitOK
}

// settleHere settles, as publish.Settle does, the publish p of the app in the current folder or
// below it whose name p names, when there is one.
func settleHere(ctx context.Context, c *platform.Client, p platform.Publish) (string, error) {
	apps, err := workspace.Apps(".")
	if err != nil {
		return "", nil
	}
	for _, a := range apps {
		if a.Name == p.App {
			return publish.Settle(ctx, c, a.Dir, p)
		}
	}
	return "", nil
}
