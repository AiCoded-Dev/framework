// Package publish is aicoded publish and aicoded status: it checks a commit of an app, sends it
// to the platform's delivery pipeline as a git bundle, and reads the outcome. git runs only with
// constant arguments, and revisions go on its standard input.
package publish

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"aicoded.dev/framework/cmd/aicoded/internal/check"
	"aicoded.dev/framework/cmd/aicoded/internal/manifest"
	"aicoded.dev/framework/cmd/aicoded/internal/platform"
	"aicoded.dev/framework/internal/errs"
)

// The longest summary: 1000 characters, and 3900 bytes of JSON, so that the meta part of an
// upload, which also holds the app's name and the commit, fits the platform's 4 KiB.
const (
	maxSummary      = 1000
	maxSummaryBytes = 3900
)

// Options are the choices of a publish.
type Options struct {
	// App, when set, must be the name of the app in the folder.
	App string
	// Summary says what changed, for the change record; "" takes the subject of the commit.
	Summary string
}

// Commit is a commit of an app that Prepare checked.
type Commit struct {
	// Dir is the app's folder, the top level of its git repository.
	Dir     string
	App     string
	SHA     string
	Summary string
}

// CheckError is E-PUB-004, with the report of aicoded check, which found problems.
type CheckError struct {
	Report check.Report
	err    error
}

func (e *CheckError) Error() string { return e.err.Error() }

func (e *CheckError) Unwrap() error { return e.err }

// Prepare checks the commit HEAD of the app in dir, before any of it leaves the computer: dir
// holds aicoded.yaml and is the top level of a git repository with a commit (E-PUB-001); git
// is 2.31 or later (E-PUB-012); nothing is left uncommitted (E-PUB-002); opts.App, when set, is
// the app's name (E-PUB-003); and aicoded check --frozen --no-tests finds no problem, with no
// replace line allowed (E-PUB-004, a *CheckError).
func Prepare(ctx context.Context, dir string, opts Options) (Commit, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return Commit{}, err
	}
	m, err := manifest.Load(filepath.Join(dir, manifest.FileName))
	if err != nil {
		return Commit{}, err
	}
	if err := checkGit(ctx); err != nil {
		return Commit{}, err
	}
	sha, err := head(ctx, dir)
	if err != nil {
		return Commit{}, err
	}
	if err := clean(ctx, dir); err != nil {
		return Commit{}, err
	}
	if opts.App != "" && opts.App != m.App {
		return Commit{}, errs.New("E-PUB-003", fmt.Sprintf("--app %q is not the app in %s, which aicoded.yaml names %q", cut(opts.App, 64), dir, m.App),
			platform.Fix("E-PUB-003"))
	}
	report, err := check.Run(ctx, dir, check.Options{Frozen: true, NoTests: true, App: m.App})
	if err != nil {
		return Commit{}, err
	}
	if !report.OK() {
		return Commit{}, &CheckError{Report: report, err: errs.New("E-PUB-004",
			"aicoded check --frozen --no-tests found problems in "+m.App+", so nothing was sent", platform.Fix("E-PUB-004"))}
	}
	summary := summarize(opts.Summary)
	if summary == "" {
		s, err := subject(ctx, dir)
		if err != nil {
			return Commit{}, err
		}
		summary = summarize(s)
	}
	return Commit{Dir: dir, App: m.App, SHA: sha, Summary: summary}, nil
}

// summarize returns s without control or format characters, with a space for each line break or
// tab, cut at 1000 characters, or before its JSON, unescaped for HTML, would pass 3900 bytes.
func summarize(s string) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		switch {
		case unicode.IsSpace(r):
			return ' '
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r), r == utf8.RuneError:
			return -1
		}
		return r
	}, s))
	runes, size := 0, 0
	for i, r := range s {
		size += utf8.RuneLen(r)
		if r == '"' || r == '\\' {
			size++
		}
		if runes == maxSummary || size > maxSummaryBytes {
			s = s[:i]
			break
		}
		runes++
	}
	return strings.TrimSpace(s)
}

// cut returns the first n characters of s.
func cut(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// Sent is a publish the platform took.
type Sent struct {
	ID  string
	App string
	SHA string
	// CreatedApp is set when the publish created the app, which the builder now owns.
	CreatedApp bool
}

// Send sends the commit to the platform's delivery pipeline: the history since the app's base,
// which it asks the platform for, or all of it. An archived app is E-PUB-006, and a commit the
// app has published already E-PUB-007. When another publish changes the base meanwhile, it asks
// again and sends once more.
func Send(ctx context.Context, c *platform.Client, cm Commit) (Sent, error) {
	for retried := false; ; retried = true {
		app, _, err := c.App(ctx, cm.App)
		if err != nil {
			return Sent{}, err
		}
		switch {
		case app.Archived:
			return Sent{}, errs.New("E-PUB-006", "the app "+cm.App+" is archived on the platform", platform.Fix("E-PUB-006"))
		case cm.SHA == app.Base || app.Last != nil && cm.SHA == app.Last.SHA:
			return Sent{}, errs.New("E-PUB-007", fmt.Sprintf("%s already published the commit %s", cm.App, short(cm.SHA)),
				platform.Fix("E-PUB-007"))
		}
		b, err := makeBundle(ctx, cm.Dir, cm.App, cm.SHA, app.Base)
		if err != nil {
			return Sent{}, err
		}
		created, err := c.CreatePublish(ctx, platform.Upload{App: cm.App, SHA: cm.SHA, Summary: cm.Summary, Bundle: b})
		switch {
		case errors.Is(err, platform.ErrBaseChanged) && !retried:
			continue
		case err != nil:
			return Sent{}, err
		}
		return Sent{ID: created.ID, App: cm.App, SHA: cm.SHA, CreatedApp: created.CreatedApp}, nil
	}
}

// ErrTimeout is the error of Wait when the publish has not ended in time.
var ErrTimeout = errors.New("the publish has not ended in time")

// Wait asks the platform for the publish id every every, until it ends, ctx ends or limit has
// passed, which is ErrTimeout. It writes each status before the end to progress, and returns
// the last answer. A request that fails in a way that may pass, such as one with no answer, is
// made again at the next turn, and the fifth such failure in a row ends the wait.
func Wait(ctx context.Context, c *platform.Client, id string, every, limit time.Duration, progress io.Writer) (platform.Publish, error) {
	deadline := time.Now().Add(limit)
	var last platform.Publish
	for failed := 0; ; {
		p, err := c.Publish(ctx, id)
		switch code := errs.Code(err); {
		case err == nil:
			failed = 0
			if p.Done() {
				return p, nil
			}
			if p.Status != last.Status {
				_, _ = fmt.Fprintf(progress, "%s ...\n", p.Status)
			}
			last = p
		case ctx.Err() != nil:
			return last, ctx.Err()
		case code != "" && code != "E-CLI-009", failed == 4:
			return last, err
		default:
			failed++
		}
		if !time.Now().Before(deadline) {
			return last, ErrTimeout
		}
		t := time.NewTimer(every)
		select {
		case <-ctx.Done():
			t.Stop()
			return last, ctx.Err()
		case <-t.C:
		}
	}
}

// Outcome returns the error of a publish whose commit the platform could not read, E-PUB-014,
// or whose delivery pipeline could not finish, E-PUB-011, and otherwise nil.
func Outcome(p platform.Publish) error {
	reason := ""
	if p.Reason != "" {
		reason = ": " + p.Reason
	}
	switch p.Status {
	case "refused":
		return errs.New("E-PUB-014", fmt.Sprintf("the platform could not read the commit %s of publish %s%s", short(p.SHA), p.ID, reason),
			platform.Fix("E-PUB-014"))
	case "error":
		return errs.New("E-PUB-011", fmt.Sprintf("the delivery pipeline could not finish publish %s%s", p.ID, reason), platform.Fix("E-PUB-011"))
	}
	return nil
}

// short returns the short form of the commit sha.
func short(sha string) string { return sha[:min(len(sha), 7)] }
