// Package describe runs aicoded describe: a compact summary of what each app of a workspace
// serves, calls and may reach, from its permission list, of its surface, of what is out of date
// in it, of its local setup in dev.yaml, never a value of it, and, in JSON, of its pages and
// layouts.
package describe

import (
	"context"
	"errors"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
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
	Bindings   []Binding           `json:"bindings,omitempty"`
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
	// Routes lists every page and layout, sorted by path, from the templates as aicoded generate
	// reads them. The text summary leaves them out.
	Routes []generate.RouteInfo `json:"routes,omitempty"`
	// Local is the app's local setup for aicoded dev.
	Local Local `json:"local"`
}

// Local is what dev.yaml sets for an app in aicoded dev: names, and whether each value is set,
// never a value.
type Local struct {
	// Workspace is the folder described when dev.yaml holds settings for it, as aicoded dev run
	// there uses them, or "" when it holds none, and aicoded dev runs with its defaults.
	Workspace string `json:"workspace"`
	// Note says what aicoded dev does when dev.yaml holds no settings for the folder, and names
	// the nearest folder above it that dev.yaml holds settings for.
	Note string `json:"note,omitempty"`
	// Problem is why aicoded dev cannot read dev.yaml, or nil.
	Problem *problem.Problem `json:"problem,omitempty"`
	// MySQL is whether dev.yaml names a local MySQL server, for an app that declares sqldb.
	MySQL *bool `json:"mysql,omitempty"`
	// Personas are the personas dev.yaml names. When it names none, aicoded dev signs in as
	// all-roles, with every role the apps ask for, or as no-roles.
	Personas []Persona `json:"personas,omitempty"`
	// Settings and Secrets are those the permission list declares, each with whether dev.yaml
	// sets a local value for it.
	Settings []Value `json:"settings,omitempty"`
	Secrets  []Value `json:"secrets,omitempty"`
}

// Persona is a test viewer of aicoded dev: its name, roles and groups.
type Persona struct {
	Name   string   `json:"name"`
	Roles  []string `json:"roles,omitempty"`
	Groups []string `json:"groups,omitempty"`
}

// Value is a setting or secret by name, and whether a local value is set for it.
type Value struct {
	Name string `json:"name"`
	Set  bool   `json:"set"`
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

// Binding is a key the runner adds to every request to an outside service: the secret's name,
// never its value, the header it goes in and the text before it.
type Binding struct {
	Host   string `json:"host"`
	Secret string `json:"secret"`
	Header string `json:"header"`
	Prefix string `json:"prefix,omitempty"`
}

// Page is one page of the access section: its path, the role rules a viewer must meet, whether a
// Guard on its path decides who sees each record, whether everyone the rules admit may see every
// record, and the page calls it makes.
type Page struct {
	Path    string   `json:"path"`
	Require []string `json:"require"`
	Guard   bool     `json:"guard,omitempty"`
	Shared  bool     `json:"shared,omitempty"`
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
	ws, local := devSetup(root)
	var s Summary
	for _, a := range apps {
		d, err := describeApp(ctx, a, names)
		if err != nil {
			return Summary{}, err
		}
		d.Local = localSetup(d, ws, local)
		s.Apps = append(s.Apps, d)
	}
	return s, nil
}

// defaults says what aicoded dev runs a folder with when dev.yaml holds no settings for it.
const defaults = "aicoded dev runs the apps here with its defaults: no local MySQL server, no local values, " +
	"and the personas all-roles and no-roles; howto E-DEV-002 shows the format of dev.yaml"

// noWorkspace is the note of a folder that dev.yaml holds no settings for, nor for a folder
// above it.
const noWorkspace = "dev.yaml names no workspace at or above this folder, so " + defaults

// aboveWorkspace returns the note of a folder that dev.yaml holds no settings for, but holds
// them for the folder above it.
func aboveWorkspace(above string) string {
	return "dev.yaml names the workspace " + above + ", not this folder; aicoded dev uses its values only when it runs there, so " + defaults
}

// devSetup returns the settings in dev.yaml that aicoded dev run in root uses, and the part of
// the local setup that every app shares: the workspace, its personas, or the problem that stops
// aicoded dev from reading dev.yaml.
func devSetup(root string) (devconfig.Workspace, Local) {
	path, err := devconfig.DefaultPath()
	if err != nil {
		return devconfig.Workspace{}, Local{Problem: &problem.Problem{Message: "aicoded cannot find its config folder: " + err.Error()}}
	}
	ws, found, above, err := devconfig.Find(path, root)
	if err != nil {
		ps := problem.From("", err)
		return devconfig.Workspace{}, Local{Problem: &ps[0]}
	}
	var local Local
	switch {
	case found:
		local.Workspace = root
	case above != "":
		local.Note = aboveWorkspace(above)
	default:
		local.Note = noWorkspace
	}
	for _, p := range ws.Personas {
		local.Personas = append(local.Personas, Persona{Name: p.Name, Roles: p.Roles, Groups: p.Groups})
	}
	return ws, local
}

// localSetup returns the local setup of the app d: the shared part local, and whether ws sets the
// MySQL server it needs and the values of its settings and secrets.
func localSetup(d App, ws devconfig.Workspace, local Local) Local {
	if local.Problem != nil {
		return local
	}
	if d.SQLDB {
		set := ws.MySQL != ""
		local.MySQL = &set
	}
	values := ws.Apps[d.Name]
	for _, name := range d.Settings {
		_, set := values.Settings[name]
		local.Settings = append(local.Settings, Value{Name: name, Set: set})
	}
	for _, name := range d.Secrets {
		_, set := values.Secrets[name]
		local.Secrets = append(local.Secrets, Value{Name: name, Set: set})
	}
	return local
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
		Egress:     m.EgressHosts(),
		Settings:   m.Settings,
		Secrets:    m.Secrets,
		Modules:    m.Modules,
		Size:       m.Size,
		Resources:  m.Resources,
		TTL:        m.TTL,
		Warnings:   res.Warnings,
		Findings:   problem.From(a.Name, err),
		Routes:     res.Routes,
	}
	for _, host := range d.Egress {
		if b, ok := m.Binding(host); ok {
			d.Bindings = append(d.Bindings, Binding{Host: b.Host, Secret: b.Secret, Header: b.Header, Prefix: b.Prefix})
		}
	}
	if len(m.Audience.Internal) > 0 || len(m.Audience.External) > 0 {
		d.Audience = &Audience{Internal: m.Audience.Internal, External: m.Audience.External}
	}
	for _, path := range slices.Sorted(maps.Keys(m.Access)) {
		access := m.Access[path]
		d.Pages = append(d.Pages, Page{Path: path, Require: access.Require, Guard: access.Guard, Shared: access.Shared, Calls: access.Calls})
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
