package generate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"aicoded.dev/framework/cmd/aicoded/internal/rpcschema"
	"aicoded.dev/framework/cmd/aicoded/internal/workspace"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/manifest"
)

// CheckSnapshots checks every snapshot of a called app that an app of the workspace holds,
// .aicoded/services/<app>.json, against the called app's current .aicoded/rpc.json. apps maps
// each app's name to its folder. It fails with E-RPC-013 at the first held snapshot the called
// app breaks, and returns one warning for each held app that is not in apps.
func CheckSnapshots(apps map[string]string) ([]string, error) {
	var warnings []string
	for _, caller := range slices.Sorted(maps.Keys(apps)) {
		w, err := CheckHeld(caller, apps)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, w...)
	}
	return warnings, nil
}

// CheckHeld checks the snapshots that the app named app holds, as CheckSnapshots does for every
// app of apps.
func CheckHeld(app string, apps map[string]string) ([]string, error) {
	held, err := HeldApps(apps[app])
	if err != nil {
		return nil, err
	}
	var warnings []string
	for _, callee := range held {
		dir, ok := apps[callee]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("%s holds a snapshot of %s, which is not in this workspace", app, callee))
			continue
		}
		if err := checkSnapshot(app, apps[app], callee, dir); err != nil {
			return nil, err
		}
	}
	return warnings, nil
}

// checkSnapshot checks the snapshot of app that caller, in callerDir, holds against the current
// snapshot of app, in appDir. An app without a current snapshot serves no function.
func checkSnapshot(caller, callerDir, app, appDir string) error {
	path, data, held, err := readHeld(callerDir, app)
	if err != nil {
		return err
	}
	current := rpcschema.Schema{App: app}
	name := filepath.Join(".aicoded", "rpc.json")
	cur, err := readIn(appDir, name)
	if err == nil {
		current, err = rpcschema.Parse(filepath.Join(appDir, name), cur)
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return broken(caller, app, path, data, rpcschema.Compatible(held, current),
		"restore both the rpc/ folder and .aicoded/rpc.json of "+app+" from the version history, or run aicoded rpc add "+app+" in "+caller+" and update its calls")
}

// compatible refuses, with E-RPC-013, a new schema of the app's functions that breaks a snapshot
// of them held by another app of the workspace: the apps of workspace, by name, or with nil the
// apps in the folders next to the app's folder. It runs before anything is written, so the app's
// .aicoded/rpc.json never records a change that breaks a caller. An app without a permission
// list has no name, so no app holds a snapshot of it.
func (g *gen) compatible(workspace map[string]string) error {
	path := filepath.Join(g.app.Dir, manifest.FileName)
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	m, err := manifest.Load(path)
	if err != nil {
		return err
	}
	callers := workspace
	if callers == nil {
		if callers, err = holders(g.app.Dir, m.App); err != nil {
			return err
		}
	}
	current := rpcschema.Schema{App: m.App}
	if g.schema != nil {
		current = *g.schema
	}
	for _, caller := range slices.Sorted(maps.Keys(callers)) {
		if caller == m.App {
			continue
		}
		path, data, held, err := readHeld(callers[caller], m.App)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		err = broken(caller, m.App, path, data, rpcschema.Compatible(held, current),
			"undo the change to the rpc/ folder of "+m.App+", or run aicoded rpc add "+m.App+" in "+caller+" and update its calls")
		if err != nil {
			return err
		}
	}
	return nil
}

// holders returns, by name, the apps in the folders next to dir that hold a snapshot of app. A
// folder without a permission list, or one that cannot be read, is not an app. It fails with
// E-DEV-010 when two of them have one name.
func holders(dir, app string) (map[string]string, error) {
	parent := filepath.Dir(dir)
	if parent == dir {
		return nil, nil
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		return nil, err
	}
	apps := map[string]string{}
	for _, e := range entries {
		sub := filepath.Join(parent, e.Name())
		if !e.IsDir() || sub == dir {
			continue
		}
		if _, err := os.Lstat(filepath.Join(sub, manifest.FileName)); err != nil {
			continue
		}
		if _, err := os.Lstat(filepath.Join(sub, ".aicoded", "services", app+".json")); err != nil {
			continue
		}
		m, err := manifest.Load(filepath.Join(sub, manifest.FileName))
		if err != nil {
			return nil, err
		}
		if other, ok := apps[m.App]; ok {
			return nil, workspace.SameName(m.App, other, sub)
		}
		apps[m.App] = sub
	}
	return apps, nil
}

// readHeld reads the snapshot of app that the app in dir holds, and returns its path, its content
// and its schema.
func readHeld(dir, app string) (string, []byte, rpcschema.Schema, error) {
	name := filepath.Join(".aicoded", "services", app+".json")
	data, err := readIn(dir, name)
	if err != nil {
		return "", nil, rpcschema.Schema{}, err
	}
	path := filepath.Join(dir, name)
	s, err := rpcschema.Parse(path, data)
	return path, data, s, err
}

// broken returns nil when breaks is empty, and otherwise E-RPC-013 with fix at the first break,
// in the snapshot of app that caller holds at path, whose content is data.
func broken(caller, app, path string, data []byte, breaks []rpcschema.Break, fix string) error {
	if len(breaks) == 0 {
		return nil
	}
	b := breaks[0]
	msg := fmt.Sprintf("%s holds a snapshot of %s that %s no longer matches: %s", caller, app, app, strings.TrimSpace(b.Method+" "+b.Path))
	if b.Reason != "" {
		msg += ": " + b.Reason
	}
	if len(breaks) > 1 {
		msg += fmt.Sprintf(" (and %d more)", len(breaks)-1)
	}
	return errs.At(fmt.Sprintf("%s:%d", path, methodLine(data, b.Method)), "E-RPC-013", msg, fix)
}

// readIn reads the file name inside dir.
func readIn(dir, name string) ([]byte, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.ReadFile(name)
}

// methodLine returns the line of the key method in the top-level "methods" object of the
// snapshot data, or 1 when there is none.
func methodLine(data []byte, method string) int {
	type level struct {
		object, key bool
		name        string
	}
	var stack []*level
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			return 1
		}
		var top *level
		if len(stack) > 0 {
			top = stack[len(stack)-1]
		}
		switch t := tok.(type) {
		case json.Delim:
			if t == '{' || t == '[' {
				stack = append(stack, &level{object: t == '{', key: t == '{'})
				continue
			}
			stack = stack[:len(stack)-1]
			if n := len(stack); n > 0 && stack[n-1].object {
				stack[n-1].key = true
			}
			continue
		case string:
			if top != nil && top.key {
				top.name, top.key = t, false
				if len(stack) == 2 && stack[0].name == "methods" && t == method {
					return 1 + bytes.Count(data[:dec.InputOffset()], []byte("\n"))
				}
				continue
			}
		}
		if top != nil && top.object {
			top.key = true
		}
	}
}
