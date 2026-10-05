package dev

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/manifest"
	"aicoded.dev/framework/cmd/aicoded/internal/problem"
	"aicoded.dev/framework/cmd/aicoded/internal/watch"
	"aicoded.dev/framework/cmd/aicoded/internal/workspace"
)

// quiet is how long aicoded dev waits after a change for more before it rebuilds.
const quiet = 300 * time.Millisecond

// Skip reports whether aicoded dev ignores the folder rel under a workspace root: a folder whose
// name starts with . or _, other than .aicoded, vendor, node_modules, testdata and the assets
// aicoded generate writes, pages/assets_gen.
func Skip(rel string) bool {
	name := path.Base(rel)
	switch {
	case rel == "" || rel == "." || name == ".aicoded":
		return false
	case strings.HasPrefix(name, "."), strings.HasPrefix(name, "_"), name == "vendor", name == "node_modules", name == "testdata":
		return true
	}
	return name == "assets_gen" && path.Base(path.Dir(rel)) == "pages"
}

// startWatching watches the workspace root and rebuilds the apps whose files change, until
// Close. An app changed since its cycle is rebuilt at once. It watches the root with its
// symbolic links resolved, as the folders of the apps are.
func (w *Workspace) startWatching() {
	root, err := filepath.EvalSymlinks(w.cfg.Root)
	var wt *watch.Watcher
	if err == nil {
		wt, err = watch.New(root, Skip, quiet)
	}
	if err != nil {
		fmt.Fprintf(w.out, "aicoded dev: cannot watch %s: %v\n", w.cfg.Root, err)
		return
	}
	w.watcher = wt
	w.background.Add(1)
	go func() {
		defer w.background.Done()
		errs := wt.Errors()
		for {
			select {
			case paths, ok := <-wt.Changes():
				if !ok {
					return
				}
				w.changed(paths)
			case err, ok := <-errs:
				if !ok {
					errs = nil
					continue
				}
				w.watchError(err)
			}
		}
	}()
	for _, s := range w.allSlots() {
		w.kick(s)
	}
}

// watchError prints err, an error of the watcher. When the watcher lost changes, it rescans the
// workspace and checks every app for a change.
func (w *Workspace) watchError(err error) {
	fmt.Fprintf(w.out, "aicoded dev: %v\n", err)
	if errors.Is(err, fsnotify.ErrEventOverflow) {
		w.Rescan(w.ctx)
		for _, s := range w.allSlots() {
			w.kick(s)
		}
	}
}

// changed handles a burst of changed paths: it rescans the workspace when apps may have come
// or gone, and checks each app that holds one of the paths for a change.
func (w *Workspace) changed(paths []string) {
	if slices.ContainsFunc(paths, w.rescans) {
		w.Rescan(w.ctx)
	}
	for _, s := range w.allSlots() {
		if slices.ContainsFunc(paths, func(p string) bool { return within(s.dir, p) }) {
			w.kick(s)
		}
	}
}

// rescans reports whether the change of p may add or remove an app: p is a permission list, a
// folder, a path that is gone or a path in no app's folder.
func (w *Workspace) rescans(p string) bool {
	if filepath.Base(p) == manifest.FileName {
		return true
	}
	if info, err := os.Lstat(p); err != nil || info.IsDir() {
		return true
	}
	return !slices.ContainsFunc(w.allSlots(), func(s *slot) bool { return within(s.dir, p) })
}

// within reports whether p is dir or lies below it.
func within(dir, p string) bool {
	return p == dir || strings.HasPrefix(p, dir+string(filepath.Separator))
}

// kick applies, in the background, what the developer asked for the app of s and a change of its
// files: now, or once more after the check that runs.
func (w *Workspace) kick(s *slot) {
	s.mu.Lock()
	if s.checking {
		s.again = true
		s.mu.Unlock()
		return
	}
	s.checking = true
	s.mu.Unlock()
	w.background.Add(1)
	go func() {
		defer w.background.Done()
		for {
			w.apply(s)
			s.mu.Lock()
			again := s.again && w.ctx.Err() == nil
			s.again, s.checking = false, again
			s.mu.Unlock()
			if !again {
				return
			}
		}
	}()
}

// apply brings the app of s to what the developer asked for last: it stops the app, or runs a
// cycle when the developer asked for one or, while aicoded dev watches, when the fingerprint of
// its files differs from the one its last cycle recorded.
func (w *Workspace) apply(s *slot) {
	s.cycling.Lock()
	defer s.cycling.Unlock()
	s.mu.Lock()
	stopped, forced := s.stopped, s.forced
	s.forced = false
	s.mu.Unlock()
	switch {
	case w.ctx.Err() != nil || s.removed:
	case stopped:
		w.halt(s)
	case forced || w.cfg.Watch && fingerprint(s.dir) != s.fingerprint:
		w.runCycle(w.ctx, s, forced)
	}
}

// mark records the fingerprint of the files of s, when aicoded dev watches them.
func (w *Workspace) mark(s *slot) {
	if w.cfg.Watch {
		s.fingerprint = fingerprint(s.dir)
	}
}

// fingerprint returns a hash of the paths and contents of the files of the app in dir that its
// build reads: it leaves out the folders Skip names, hidden files, and the files aicoded
// generate writes other than the permission list.
func fingerprint(dir string) string {
	h := sha256.New()
	root, err := os.OpenRoot(dir)
	if err != nil {
		return ""
	}
	defer root.Close()
	_ = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return nil
		case d.IsDir():
			if p != "." && Skip(p) {
				return fs.SkipDir
			}
			return nil
		case Skip(p), !d.Type().IsRegular(), generated(p):
			return nil
		}
		data, err := root.ReadFile(filepath.FromSlash(p))
		if err != nil {
			return nil
		}
		fmt.Fprintf(h, "%s\x00%d\x00", p, len(data))
		h.Write(data)
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))
}

// generated reports whether aicoded generate writes the file rel of an app.
func generated(rel string) bool {
	return strings.HasSuffix(rel, "_gen.go") || strings.HasSuffix(rel, "_gen.ts") || rel == ".aicoded/rpc.json"
}

// Rescan starts the apps that appeared under the workspace root since it last looked, and stops
// those that are gone, whose aicoded.yaml is gone or whose name changed. A new app with the name
// of another fails with E-DEV-010 and leaves the other running. A new folder whose aicoded.yaml
// does not load starts nothing; Rescan prints its error once, and again when it changes.
func (w *Workspace) Rescan(ctx context.Context) {
	w.rescanning.Lock()
	defer w.rescanning.Unlock()
	if w.ctx.Err() != nil {
		return
	}
	dirs, err := workspace.Discover(w.cfg.Root)
	if err != nil {
		fmt.Fprintf(w.out, "aicoded dev: cannot look for apps in %s: %v\n", w.cfg.Root, err)
		return
	}
	found := map[string]*manifest.Manifest{}
	failed := map[string]string{}
	for _, dir := range dirs {
		if m, err := manifest.Load(filepath.Join(dir, manifest.FileName)); err == nil {
			found[dir] = &m
		} else {
			found[dir], failed[dir] = nil, err.Error()
		}
	}
	for _, s := range w.allSlots() {
		m, ok := found[s.dir]
		if !ok || m != nil && m.App != s.name {
			w.remove(s)
		}
	}
	unloadable := map[string]string{}
	for _, dir := range dirs {
		if slices.ContainsFunc(w.allSlots(), func(s *slot) bool { return s.dir == dir }) {
			continue
		}
		if m := found[dir]; m != nil {
			w.add(ctx, newSlot(m.App, dir, *m, w.out, w.changes))
			continue
		}
		if w.unloadable[dir] != failed[dir] {
			fmt.Fprintf(w.out, "aicoded dev: %s: %s\n", dir, failed[dir])
		}
		unloadable[dir] = failed[dir]
	}
	w.unloadable = unloadable
	for _, s := range w.allSlots() {
		if s.dup.Load() && w.taken(s) == nil {
			s.dup.Store(false)
			w.start(ctx, s)
		}
	}
}

// add puts the new slot s in the workspace and starts its app, unless another app has its name.
func (w *Workspace) add(ctx context.Context, s *slot) {
	if w.cfg.Dev.MySQL != "" {
		s.store.hideSecrets(dsnSecrets(w.cfg.Dev.MySQL))
	}
	s.manual = slices.Contains(w.cfg.Manual, s.name)
	other := w.taken(s)
	s.dup.Store(other != nil)
	w.mu.Lock()
	w.slots = append(w.slots, s)
	slices.SortFunc(w.slots, func(a, b *slot) int { return strings.Compare(a.dir, b.dir) })
	w.mu.Unlock()
	if other == nil {
		w.start(ctx, s)
		return
	}
	ps := problem.From(s.name, workspace.SameName(s.name, other.dir, s.dir))
	s.set(devapi.Failed, ps)
	w.report(s, sourceRunner, ps)
}

// start runs the first cycle of s, which joins the apps started.
func (w *Workspace) start(ctx context.Context, s *slot) {
	w.mu.Lock()
	w.started = append(w.started, s)
	w.mu.Unlock()
	w.cycle(ctx, s)
}

// taken returns the slot, other than s, that runs an app with the name of s.
func (w *Workspace) taken(s *slot) *slot {
	for _, o := range w.allSlots() {
		if o != s && !o.dup.Load() && o.name == s.name {
			return o
		}
	}
	return nil
}

// remove stops the app of s and takes s out of the workspace. An app that waits for its name
// goes silently.
func (w *Workspace) remove(s *slot) {
	s.cycling.Lock()
	s.removed = true
	if !s.dup.Load() {
		w.gateway.Remove(s.name)
	}
	w.stopInstance(s)
	s.set(devapi.Stopped, nil)
	s.cycling.Unlock()
	w.mu.Lock()
	w.slots = slices.DeleteFunc(w.slots, func(o *slot) bool { return o == s })
	w.started = slices.DeleteFunc(w.started, func(o *slot) bool { return o == s })
	w.mu.Unlock()
	w.changes.signal()
	if !s.dup.Load() {
		w.svc.Router.Remove(s.name)
		fmt.Fprintf(w.out, "aicoded dev: %s removed\n", s.name)
	}
}
