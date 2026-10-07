// Package describe runs aicoded describe: a compact summary of what each app of a workspace
// serves, calls and may reach, from its permission list, of its surface, and of what is out of
// date in it.
package describe

import (
	"context"
	"errors"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"aicoded.dev/framework/cmd/aicoded/internal/generate"
	"aicoded.dev/framework/cmd/aicoded/internal/problem"
	"aicoded.dev/framework/cmd/aicoded/internal/surface"
	"aicoded.dev/framework/cmd/aicoded/internal/workspace"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/lint"
	"aicoded.dev/framework/manifest"
)

// Summary describes the apps of a workspace.
type Summary struct {
	Apps []App `json:"apps"`
}

// App describes one app.
type App struct {
	Name       string              `json:"name"`
	Dir        string              `json:"dir"`
	Class      string              `json:"class,omitempty"`
	Owner      string              `json:"owner,omitempty"`
	Audience   *Audience           `json:"audience,omitempty"`
	Pages      []Page              `json:"pages,omitempty"`
	Serves     []Serve             `json:"serves,omitempty"`
	Calls      map[string][]string `json:"calls,omitempty"`
	SQLDB      bool                `json:"sqldb,omitempty"`
	Stores     []string            `json:"stores,omitempty"`
	Connectors []string            `json:"connectors,omitempty"`
	Email      *Email              `json:"email,omitempty"`
	Egress     []string            `json:"egress,omitempty"`
	Schedule   []Job               `json:"schedule,omitempty"`
	Settings   []string            `json:"settings,omitempty"`
	Secrets    []string            `json:"secrets,omitempty"` // names only
	Modules    []string            `json:"modules,omitempty"`
	Size       string              `json:"size,omitempty"`
	Resources  string              `json:"resources,omitempty"`
	TTL        string              `json:"ttl,omitempty"`
	// Surface is the app's size. Its writes come from lint, and are left out, with a warning, when
	// lint cannot read the app or its precheck stops it.
	Surface  surface.Surface `json:"surface"`
	Warnings []string        `json:"warnings,omitempty"`
	// Findings are the generated files that are out of date, E-CHK-001, and the errors of generate.
	Findings []problem.Problem `json:"findings,omitempty"`
}

// Audience is who may use the app: people of the company and outside people.
type Audience struct {
	Internal []string `json:"internal,omitempty"`
	External []string `json:"external,omitempty"`
}

// Job is one scheduled job and the cron expression, in UTC, of when it runs.
type Job struct {
	Name string `json:"job"`
	Cron string `json:"cron"`
}

// Page is one page of the access section: its path, the role rules a viewer must meet, whether it
// guards the pages below it, and the page calls it makes.
type Page struct {
	Path    string   `json:"path"`
	Require []string `json:"require"`
	Guard   bool     `json:"guard,omitempty"`
	Calls   []string `json:"calls,omitempty"`
}

// Serve is one function the app serves other apps.
type Serve struct {
	Name    string   `json:"name"`
	Callers []string `json:"callers"`
	Require []string `json:"require,omitempty"`
	Apps    bool     `json:"apps,omitempty"`
}

// Email is where the app sends mail from and the domains it may send to.
type Email struct {
	From      string   `json:"from"`
	ToDomains []string `json:"to_domains,omitempty"`
}

// Describe summarises the app named app under dir, or every app when app is "". It never writes.
func Describe(ctx context.Context, dir, app string) (Summary, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return Summary{}, err
	}
	all, err := workspace.Apps(root)
	if err != nil {
		return Summary{}, err
	}
	apps, err := workspace.Select(all, app)
	if err != nil {
		return Summary{}, err
	}
	names := workspace.Names(all)
	var s Summary
	for _, a := range apps {
		d, err := describeApp(ctx, a, names)
		if err != nil {
			return Summary{}, err
		}
		s.Apps = append(s.Apps, d)
	}
	return s, nil
}

// describeApp describes the app a of the workspace names.
func describeApp(ctx context.Context, a workspace.App, names map[string]string) (App, error) {
	m, err := manifest.Load(filepath.Join(a.Dir, manifest.FileName))
	if err != nil {
		return App{}, err
	}
	res, err := generate.Run(a.Dir, generate.Options{Workspace: names, Frozen: true})
	d := App{
		Name:       a.Name,
		Dir:        a.Dir,
		Class:      m.Class,
		Owner:      m.Owner,
		Calls:      m.Services.Calls,
		SQLDB:      m.SQLDB(),
		Stores:     m.Stores(),
		Connectors: m.Connectors(),
		Egress:     m.Egress,
		Settings:   m.Settings,
		Secrets:    m.Secrets,
		Modules:    m.Modules,
		Size:       m.Size,
		Resources:  m.Resources,
		TTL:        m.TTL,
		Warnings:   res.Warnings,
		Findings:   problem.From(a.Name, err),
	}
	if len(m.Audience.Internal) > 0 || len(m.Audience.External) > 0 {
		d.Audience = &Audience{Internal: m.Audience.Internal, External: m.Audience.External}
	}
	for _, path := range slices.Sorted(maps.Keys(m.Access)) {
		access := m.Access[path]
		d.Pages = append(d.Pages, Page{Path: path, Require: access.Require, Guard: access.Guard, Calls: access.Calls})
	}
	for _, name := range slices.Sorted(maps.Keys(m.Services.Serves)) {
		serve := m.Services.Serves[name]
		d.Serves = append(d.Serves, Serve{Name: name, Callers: serve.Callers, Require: serve.Require, Apps: serve.Apps})
	}
	if m.Email != nil {
		d.Email = &Email{From: m.Email.From, ToDomains: m.Email.ToDomains}
	}
	for _, j := range m.Schedule {
		d.Schedule = append(d.Schedule, Job{Name: j.Name, Cron: j.Cron})
	}
	r, err := lint.Run(ctx, a.Dir, lint.Config{Generated: res.Files, Modules: m.Modules})
	if ctx.Err() != nil {
		return App{}, ctx.Err()
	}
	switch {
	case err != nil:
		d.Warnings = append(d.Warnings, "the surface counts no SQL writes: lint cannot read the app: "+inApp(firstLine(err), a.Dir))
	case r.Stopped:
		p := r.Problems[0]
		d.Warnings = append(d.Warnings, "the surface counts no SQL writes: lint stopped before reading the app: "+p.Pos+": "+p.Code+": "+p.Msg)
	}
	d.Surface = surface.Of(m, r.Writes)
	return d, nil
}

// inApp returns msg with the paths of files in the app folder dir written relative to it.
func inApp(msg, dir string) string {
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		msg = strings.ReplaceAll(msg, resolved+string(filepath.Separator), "")
	}
	return strings.ReplaceAll(msg, dir+string(filepath.Separator), "")
}

// firstLine returns the message of err, without its fix when it is coded, up to its first line
// break.
func firstLine(err error) string {
	msg := err.Error()
	var e *errs.Error
	if errors.As(err, &e) {
		msg = e.Msg
	}
	msg, _, _ = strings.Cut(msg, "\n")
	return msg
}
