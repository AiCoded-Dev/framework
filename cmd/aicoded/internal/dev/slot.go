package dev

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/cmd/aicoded/internal/generate"
	"aicoded.dev/framework/cmd/aicoded/internal/manifest"
	"aicoded.dev/framework/cmd/aicoded/internal/problem"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/runnerproto"
)

// slot is one app of the workspace. It outlives the app's instances: a restart replaces the
// instance and keeps the slot.
type slot struct {
	name, dir   string
	store       *store
	dup         atomic.Bool // another app has the name; this one waits until it is free
	cycling     sync.Mutex  // held while the app is generated, built, started or stopped
	builds      int         // guarded by cycling
	announced   bool        // guarded by cycling
	removed     bool        // guarded by cycling
	fingerprint string      // guarded by cycling

	mu        sync.Mutex // guards the fields below
	m         manifest.Manifest
	state     devapi.State
	problems  []problem.Problem
	since     time.Time
	callsOnly bool
	inst      *Instance
	checking  bool   // a check of the files runs
	again     bool   // another check follows it
	stopped   bool   // the developer stopped the app; it stays stopped until started
	forced    bool   // the developer asked for a cycle that has not begun
	manual    bool   // the developer runs the app by hand
	runDir    string // the run folder while the app runs by hand; changed only under cycling
}

// newSlot returns the slot of the app name in dir, whose permission list is m. The app's inbox
// takes mail by the mail rules of m from now on, before the app first runs. Its state, log
// entries, spans and mail are signalled on ch, which may be nil.
func newSlot(name, dir string, m manifest.Manifest, out io.Writer, ch *changes) *slot {
	st := newStore(name, out)
	st.changes, st.mail.changes = ch, ch
	st.mail.setEmail(m.Email)
	return &slot{name: name, dir: dir, store: st, m: m, state: devapi.Starting, since: time.Now(), callsOnly: callsOnly(m)}
}

// set puts the slot in state with problems. Since changes only with the state.
func (s *slot) set(state devapi.State, problems []problem.Problem) {
	s.mu.Lock()
	if state != s.state {
		s.since = time.Now()
	}
	s.state, s.problems = state, problems
	s.mu.Unlock()
	s.store.changes.signal()
}

// cycle generates, builds and starts the app of s, and puts the new instance in place of the
// running one. When a step fails, the old instance stops and the app's host shows the problems.
func (w *Workspace) cycle(ctx context.Context, s *slot) {
	s.cycling.Lock()
	defer s.cycling.Unlock()
	w.runCycle(ctx, s, false)
}

// runCycle is cycle for a caller that holds s.cycling. It does nothing for a slot that was
// removed, waits for its name or was stopped by the developer, nor once ctx is done. The
// gateway keeps serving the running instance until the new one is built. For an app run by
// hand, it only generates and keeps the runner up, and the app stays manual; renew brings up a
// new runner.
func (w *Workspace) runCycle(ctx context.Context, s *slot, renew bool) {
	s.mu.Lock()
	stopped, byHand, running := s.stopped, s.manual, s.inst != nil
	regenerate := byHand && !renew && s.state == devapi.Manual
	s.mu.Unlock()
	if s.removed || s.dup.Load() || stopped || ctx.Err() != nil {
		return
	}
	if !regenerate {
		s.set(devapi.Starting, nil)
	}
	if !running {
		w.gateway.Starting(s.name)
	}
	m, values, bin, err := w.prepare(ctx, s, byHand)
	if err != nil {
		w.failed(ctx, s, sourceBuild, err)
		return
	}
	if byHand {
		w.runByHand(ctx, s, m, values, renew)
		return
	}
	w.gateway.Starting(s.name)
	w.stopInstance(s)
	inst, err := launch(ctx, s.dir, bin, m, values, w.svc, w.signer, s.store)
	if err != nil {
		w.failed(ctx, s, sourceRunner, err)
		return
	}
	removeBins(filepath.Dir(bin), s.name, filepath.Base(bin))
	s.mu.Lock()
	s.inst, s.callsOnly = inst, callsOnly(m)
	s.mu.Unlock()
	w.svc.Router.Add(m, inst.AppSocket())
	if callsOnly(m) {
		w.gateway.Remove(s.name)
	} else {
		w.gateway.Add(s.name, inst.AppSocket(), s.store)
	}
	s.set(devapi.Running, nil)
	if s.announced {
		fmt.Fprintf(w.out, "aicoded dev: %s restarted\n", s.name)
	} else {
		fmt.Fprintln(w.out, readyLine(s.name, callsOnly(m), w.port))
		s.announced = true
	}
	w.watchExit(s, inst)
}

// prepare runs the steps of a cycle before the old instance stops: the dev.yaml values,
// generate, the held snapshots and, unless the app runs by hand, the build. It returns the
// permission list, the values and the built app.
func (w *Workspace) prepare(ctx context.Context, s *slot, byHand bool) (manifest.Manifest, devconfig.AppValues, string, error) {
	values, err := w.values(s.name)
	if err != nil {
		w.mark(s)
		return manifest.Manifest{}, values, "", err
	}
	s.store.hideSecrets(values.Secrets)
	names := w.names()
	m, err := generateApp(s.dir, names)
	w.mark(s)
	if err != nil {
		return m, values, "", err
	}
	if m.App != s.name {
		path := filepath.Join(s.dir, manifest.FileName)
		return m, values, "", errs.New("E-DEV-015", fmt.Sprintf("the app: line of %s names %s now, not %s", path, m.App, s.name),
			fmt.Sprintf("change app: in %s back to %s, or wait for aicoded dev to restart the app under its new name", path, s.name))
	}
	s.mu.Lock()
	s.m = m
	s.mu.Unlock()
	if w.personas {
		w.gateway.setPersonas(DefaultPersonas(w.manifests()...))
	}
	warnings, err := generate.CheckHeld(s.name, names)
	for _, warning := range warnings {
		fmt.Fprintln(w.out, "aicoded dev: "+warning)
	}
	if err != nil {
		return m, values, "", err
	}
	if err := checkStart(m, values, w.svc); err != nil {
		return m, values, "", err
	}
	for _, name := range shortSecrets(m, values) {
		line := fmt.Sprintf("aicoded dev: secret %s of %s is shorter than %d bytes, so its value is not hidden", name, s.name, minSecret)
		fmt.Fprintln(w.out, line)
		s.store.add(sourceRunner, "WARN", line, false)
	}
	if byHand {
		return m, values, "", nil
	}
	s.builds++
	bin := filepath.Join(w.cfg.State, "bin", fmt.Sprintf("%s.%d", s.name, s.builds))
	return m, values, bin, build(ctx, s.dir, bin)
}

// failed ends a cycle that err, from source, stopped: as a failure, or, when ctx is done
// because aicoded dev is stopping, with the app stopped.
func (w *Workspace) failed(ctx context.Context, s *slot, source string, err error) {
	if ctx.Err() != nil {
		w.stopInstance(s)
		s.set(devapi.Stopped, nil)
		return
	}
	w.fail(s, source, problem.From(s.name, err))
}

// fail stops the app of s, and shows ps, with the secret values hidden, at its host. It keeps
// them as ERROR entries from source and prints them.
func (w *Workspace) fail(s *slot, source string, ps []problem.Problem) {
	hide := s.store.redactor()
	for i, p := range ps {
		ps[i].Pos, ps[i].Message, ps[i].Fix = hide.String(p.Pos), hide.String(p.Message), hide.String(p.Fix)
	}
	w.gateway.Fail(s.name, ps)
	w.stopInstance(s)
	s.set(devapi.Failed, ps)
	w.report(s, source, ps)
}

// report keeps ps as ERROR entries of s from source and prints them.
func (w *Workspace) report(s *slot, source string, ps []problem.Problem) {
	var b strings.Builder
	fmt.Fprintf(&b, "aicoded dev: %s failed:\n", s.name)
	for _, p := range ps {
		p.App = ""
		s.store.add(source, "ERROR", p.String(), false)
		for line := range strings.Lines(p.String()) {
			b.WriteString("  " + strings.TrimSuffix(line, "\n") + "\n")
		}
	}
	_, _ = io.WriteString(w.out, b.String())
}

// runByHand keeps the runner of the app of s up in the run folder of manual mode, for a process
// the developer starts. It keeps the running runner, with its CSRF key and database, unless
// renew is set or the permission list m or the values changed. When manual mode ended since the
// cycle began, it only stops the runner and leaves the rest to the cycle that follows; once ctx
// is done, it leaves the app stopped. The caller holds s.cycling.
func (w *Workspace) runByHand(ctx context.Context, s *slot, m manifest.Manifest, values devconfig.AppValues, renew bool) {
	s.mu.Lock()
	old := s.inst
	s.mu.Unlock()
	if !renew && old != nil && old.byHand && reflect.DeepEqual(old.Manifest, m) && reflect.DeepEqual(old.svc.values, values) {
		s.set(devapi.Manual, nil)
		return
	}
	w.gateway.Starting(s.name)
	s.set(devapi.Starting, nil)
	w.stopInstance(s)
	s.mu.Lock()
	manual, dir := s.manual, s.runDir
	s.mu.Unlock()
	if ctx.Err() != nil {
		w.failed(ctx, s, sourceRunner, ctx.Err())
		return
	}
	if !manual {
		return
	}
	if dir == "" {
		var err error
		if dir, err = newRunDir(); err != nil {
			w.failed(ctx, s, sourceRunner, err)
			return
		}
		s.mu.Lock()
		s.runDir = dir
		s.mu.Unlock()
	}
	inst, err := launchByHand(ctx, dir, m, values, w.svc, w.signer, s.store)
	if err != nil {
		w.failed(ctx, s, sourceRunner, err)
		return
	}
	s.mu.Lock()
	s.inst, s.callsOnly = inst, callsOnly(m)
	s.mu.Unlock()
	w.svc.Router.Add(m, inst.AppSocket())
	command := manualCommand(s.dir, dir)
	if callsOnly(m) {
		w.gateway.Remove(s.name)
	} else {
		w.gateway.AddManual(s.name, inst.AppSocket(), command)
	}
	s.set(devapi.Manual, nil)
	if !s.announced {
		fmt.Fprintln(w.out, readyLine(s.name, callsOnly(m), w.port))
		s.announced = true
	}
	fmt.Fprintf(w.out, "aicoded dev: %s runs by hand; start it with: %s\n", s.name, command)
}

// manualCommand is the shell command that runs the app in dir by hand, with its runner in runDir.
func manualCommand(dir, runDir string) string {
	return "cd " + shellQuote(dir) + " && " + runnerproto.EnvRunnerDir + "=" + shellQuote(runDir) + " go run ."
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// shellQuote quotes s for a POSIX shell, unless it holds only characters that need no quotes.
func shellQuote(s string) string {
	if shellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// stopInstance takes the app of s out of the calls between apps and stops its instance. The run
// folder of manual mode stays while the app runs by hand; it goes once the app leaves manual
// mode or is removed, or aicoded dev stops. The caller holds s.cycling.
func (w *Workspace) stopInstance(s *slot) {
	s.mu.Lock()
	inst, dir := s.inst, s.runDir
	s.inst = nil
	keep := s.manual && !s.removed && w.ctx.Err() == nil
	if !keep {
		s.runDir = ""
	}
	s.mu.Unlock()
	if inst != nil {
		w.svc.Router.Remove(s.name)
		inst.Stop()
	}
	if dir != "" && !keep {
		_ = os.RemoveAll(dir)
	}
}

// halt stops the app of s and shows that at its host, until the developer starts it again.
// The caller holds s.cycling.
func (w *Workspace) halt(s *slot) {
	s.mu.Lock()
	was := s.state
	s.mu.Unlock()
	if was == devapi.Stopped {
		w.stopInstance(s)
		return
	}
	w.gateway.Stopped(s.name)
	w.stopInstance(s)
	s.set(devapi.Stopped, nil)
	fmt.Fprintf(w.out, "aicoded dev: %s stopped\n", s.name)
}

// watchExit marks the slot failed with E-DEV-004 when inst ends while it is the slot's
// instance and aicoded dev is not stopping.
func (w *Workspace) watchExit(s *slot, inst *Instance) {
	w.exits.Add(1)
	go func() {
		defer w.exits.Done()
		<-inst.Exited()
		s.cycling.Lock()
		defer s.cycling.Unlock()
		s.mu.Lock()
		current := s.inst == inst && w.ctx.Err() == nil
		s.mu.Unlock()
		if current {
			w.fail(s, sourceRunner, problem.From(s.name, errs.New("E-DEV-004", s.name+" exited", "read the app's log lines above and fix the app")))
		}
	}()
}

// generateApp checks the permission list of the app in dir, runs aicoded generate with the
// apps of the workspace ws and returns the permission list with the sections generate wrote.
func generateApp(dir string, ws map[string]string) (manifest.Manifest, error) {
	path := filepath.Join(dir, manifest.FileName)
	if _, err := manifest.Load(path); err != nil {
		return manifest.Manifest{}, err
	}
	if _, err := generate.Run(dir, generate.Options{Workspace: ws}); err != nil {
		return manifest.Manifest{}, err
	}
	return manifest.Load(path)
}

// removeBins removes the binaries of app in dir other than keep.
func removeBins(dir, app, keep string) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), app+".") && e.Name() != keep {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// readyLine is the line aicoded dev prints when app first runs.
func readyLine(app string, callsOnly bool, port int) string {
	if callsOnly {
		return fmt.Sprintf("aicoded dev: %s (calls only)", app)
	}
	base := fmt.Sprintf("http://%s.localhost:%d", app, port)
	return fmt.Sprintf("aicoded dev: %s at %s/ (switch persona at %s%s)", app, base, base, personaPath)
}

// callsOnly reports whether the app of m has no pages and serves calls, so no viewer opens it.
func callsOnly(m manifest.Manifest) bool {
	return len(m.Access) == 0 && len(m.Services.Serves) > 0
}

// DefaultPersonas are the personas of a workspace whose dev.yaml names none: one with every
// role the apps' pages and functions ask for, and one with no role.
func DefaultPersonas(ms ...manifest.Manifest) []devconfig.Persona {
	var roles []string
	for _, m := range ms {
		roles = append(roles, m.Roles()...)
	}
	slices.Sort(roles)
	return []devconfig.Persona{{Name: "all-roles", Roles: slices.Compact(roles)}, {Name: "no-roles"}}
}

// startOrder returns the indexes of ms in the order their apps start: each app after the apps of
// ms it calls, and otherwise in folder order. Of apps that call each other, directly or through
// others, the first in folder order starts first.
func startOrder(ms []manifest.Manifest) []int {
	index := make(map[string]int, len(ms))
	for i, m := range ms {
		index[m.App] = i
	}
	callees := make([][]int, len(ms))
	for i, m := range ms {
		for app := range m.Services.Calls {
			if j, ok := index[app]; ok && j != i {
				callees[i] = append(callees[i], j)
			}
		}
	}
	started := make([]bool, len(ms))
	ready := func(i int) bool {
		return !slices.ContainsFunc(callees[i], func(j int) bool { return !started[j] })
	}
	// onCycle reports whether the app i calls itself through apps not started yet.
	onCycle := func(i int) bool {
		seen := make([]bool, len(ms))
		next := slices.Clone(callees[i])
		for len(next) > 0 {
			j := next[len(next)-1]
			next = next[:len(next)-1]
			if j == i {
				return true
			}
			if !started[j] && !seen[j] {
				seen[j] = true
				next = append(next, callees[j]...)
			}
		}
		return false
	}
	first := func(ok func(int) bool) int {
		for i := range ms {
			if !started[i] && ok(i) {
				return i
			}
		}
		return -1
	}
	order := make([]int, 0, len(ms))
	for range ms {
		i := first(ready)
		if i < 0 {
			i = first(onCycle)
		}
		started[i] = true
		order = append(order, i)
	}
	return order
}
