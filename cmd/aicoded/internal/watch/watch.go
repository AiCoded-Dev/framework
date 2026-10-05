// Package watch reports the files that change below a folder, a burst of changes at a time.
package watch

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher reports the changes below a folder.
type Watcher struct {
	fs      *fsnotify.Watcher
	root    string
	skip    func(rel string) bool
	quiet   time.Duration
	changes chan []string
	errs    chan error
	done    chan struct{}
	stopped chan struct{}
}

// New watches root and every folder below it for which skip(rel) is false, where rel is the
// slash-separated path relative to root. It adds folders as they appear, and sends the
// changed paths of each burst once no change has come for quiet.
func New(root string, skip func(rel string) bool, quiet time.Duration) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := fw.Add(root); err != nil {
		_ = fw.Close()
		return nil, err
	}
	w := &Watcher{fs: fw, root: root, skip: skip, quiet: quiet, changes: make(chan []string),
		errs: make(chan error, 16), done: make(chan struct{}), stopped: make(chan struct{})}
	w.addTree(root, nil)
	go w.run()
	return w, nil
}

// Changes gets the changed paths of each burst: absolute, sorted and each once. It is closed
// when the watcher stops.
func (w *Watcher) Changes() <-chan []string { return w.changes }

// Errors gets the folders that cannot be watched and the watcher's own failures; it drops
// them while nobody reads. It is closed when the watcher stops.
func (w *Watcher) Errors() <-chan error { return w.errs }

// Close stops the watcher and waits until Changes is closed.
func (w *Watcher) Close() error {
	select {
	case <-w.done:
	default:
		close(w.done)
	}
	err := w.fs.Close()
	<-w.stopped
	return err
}

func (w *Watcher) run() {
	defer close(w.stopped)
	defer close(w.errs)
	defer close(w.changes)
	pending := map[string]bool{}
	timer := time.NewTimer(w.quiet)
	timer.Stop()
	var ready []string
	var out chan []string
	for {
		select {
		case ev, ok := <-w.fs.Events:
			if !ok {
				return
			}
			if ev.Op == fsnotify.Chmod || w.skipped(ev.Name) {
				continue
			}
			pending[ev.Name] = true
			if info, err := os.Lstat(ev.Name); err == nil && info.IsDir() && ev.Has(fsnotify.Create) {
				w.addTree(ev.Name, pending)
			}
			timer.Reset(w.quiet)
		case err, ok := <-w.fs.Errors:
			if !ok {
				return
			}
			w.fail(err)
		case <-timer.C:
			for p := range pending {
				if !slices.Contains(ready, p) {
					ready = append(ready, p)
				}
			}
			slices.Sort(ready)
			clear(pending)
			out = w.changes
		case out <- ready:
			ready, out = nil, nil
		case <-w.done:
			return
		}
	}
}

// addTree watches dir and the folders below it that are not skipped, and adds every path below
// dir to found when it is not nil.
func (w *Watcher) addTree(dir string, found map[string]bool) {
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			w.fail(fmt.Errorf("cannot watch %s: %w", p, err))
			return nil
		}
		if p != dir && w.skipped(p) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if found != nil && p != dir {
			found[p] = true
		}
		if d.IsDir() && p != w.root {
			if err := w.fs.Add(p); err != nil {
				w.fail(fmt.Errorf("cannot watch %s: %w", p, err))
			}
		}
		return nil
	})
}

// skipped reports whether skip names the path p, which lies below the root.
func (w *Watcher) skipped(p string) bool {
	rel, err := filepath.Rel(w.root, p)
	return err != nil || w.skip(filepath.ToSlash(rel))
}

func (w *Watcher) fail(err error) {
	select {
	case w.errs <- err:
	default:
	}
}
