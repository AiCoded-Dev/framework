package generate

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"aicoded.dev/framework/cmd/aicoded/internal/rpcgen"
	"aicoded.dev/framework/cmd/aicoded/internal/rpcschema"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/manifest"
)

// servicesMarker is the comment the generator writes above the services section.
const servicesMarker = "# services is written by aicoded generate from rpc/ and the clients in services/; do not edit it"

// Paths, relative to the app, of what the generator reads and writes for calls between apps.
const (
	snapshotFile = ".aicoded/rpc.json"
	serverFile   = "rpc/server_gen.go"
	// heldDir holds the snapshots of the apps this app calls, one <app>.json per app.
	heldDir = ".aicoded/services"
)

// callee generates the side of the app that serves functions to other apps from its rpc/
// package: rpc/server_gen.go, and the snapshot .aicoded/rpc.json numbered against the one there.
// It returns what the services section says the app serves. An app without rpc/ loses its
// snapshot, and one without functions its server_gen.go.
func (g *gen) callee() (map[string]manifest.Serve, error) {
	p, ok, err := rpcgen.Parse(g.app.Dir)
	if err != nil {
		return nil, err
	}
	if !ok {
		g.stale = append(g.stale, snapshotFile)
		return nil, nil
	}
	m, err := manifest.Load(filepath.Join(g.app.Dir, manifest.FileName))
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(g.app.Dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	var prev *rpcschema.Schema
	data, err := root.ReadFile(filepath.FromSlash(snapshotFile))
	switch {
	case err == nil:
		s, err := rpcschema.Parse(snapshotFile, data)
		if err != nil {
			return nil, err
		}
		prev = &s
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}
	fresh := p.Schema
	fresh.App = m.App
	s := rpcschema.Number(prev, fresh)
	g.schema = &s
	if out := s.Marshal(); !bytes.Equal(out, data) {
		g.snapshots[snapshotFile] = out
	}
	if len(p.Funcs) == 0 {
		g.stale = append(g.stale, serverFile)
		return nil, nil
	}
	code, err := rpcgen.Server(p, s)
	if err != nil {
		return nil, err
	}
	g.files[serverFile] = code
	serves := map[string]manifest.Serve{}
	for _, f := range p.Funcs {
		serves[f.Name] = manifest.Serve{Callers: f.Access.Callers, Require: f.Access.Require, Apps: f.Access.Apps}
	}
	return serves, nil
}

// callers writes the client of every app whose snapshot the app holds in .aicoded/services/,
// removes the clients of apps it no longer holds, and returns, by app, the functions of those apps
// that the app's code refers to.
func (g *gen) callers() (map[string][]string, error) {
	root, err := os.OpenRoot(g.app.Dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	held, err := loadHeld(root)
	if err != nil {
		return nil, err
	}
	names, err := rpcgen.PackageNames(slices.Collect(maps.Keys(held)))
	if err != nil {
		return nil, err
	}
	for _, app := range slices.Sorted(maps.Keys(held)) {
		code, err := rpcgen.Client(app, held[app])
		if err != nil {
			return nil, err
		}
		g.files["services/"+names[app]+"/client_gen.go"] = code
	}
	if err := onlyClients(root, names); err != nil {
		return nil, err
	}
	entries, err := fs.ReadDir(root.FS(), "services")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, e := range entries {
		rel := "services/" + e.Name() + "/client_gen.go"
		if _, ok := g.files[rel]; ok || !e.IsDir() {
			continue
		}
		if _, err := root.Lstat(filepath.FromSlash(rel)); err == nil {
			g.stale = append(g.stale, rel)
			g.emptied = append(g.emptied, "services/"+e.Name())
		}
	}
	if len(g.emptied) > 0 {
		g.emptied = append(g.emptied, "services")
	}
	if len(held) == 0 {
		return nil, nil
	}
	return rpcgen.Calls(g.app.Dir, g.app.Module, held)
}

// onlyClients refuses, with E-RPC-014, every entry other than client_gen.go in the folder
// services/<pkg>/ of the client of a held app, names mapping each held app to its package. The
// scan for calls skips services/, so code there could call other apps without the services
// section saying so.
func onlyClients(root *os.Root, names map[string]string) error {
	var d Diagnostics
	for _, app := range slices.Sorted(maps.Keys(names)) {
		dir := "services/" + names[app]
		entries, err := fs.ReadDir(root.FS(), dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.Name() == "client_gen.go" {
				continue
			}
			rel := dir + "/" + e.Name()
			d = append(d, errs.At(rel, "E-RPC-014",
				fmt.Sprintf("%s is not the generated client of %s, and the calls in %s/ are left out of the permission list", rel, app, dir),
				"move "+rel+" out of "+dir+"/; that folder holds only the generated client"))
		}
	}
	return d.Err()
}

// HeldApps returns the apps whose snapshots the app in dir holds in .aicoded/services/, by the
// names of their files, sorted. It never follows a symbolic link out of dir.
func HeldApps(dir string) ([]string, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return heldApps(root)
}

// heldApps is HeldApps for the app in root.
func heldApps(root *os.Root) ([]string, error) {
	entries, err := fs.ReadDir(root.FS(), heldDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var apps []string
	for _, e := range entries {
		if app, ok := strings.CutSuffix(e.Name(), ".json"); ok && !e.IsDir() {
			apps = append(apps, app)
		}
	}
	return apps, nil
}

// loadHeld reads the snapshots in .aicoded/services/ of the app in root, by app. It refuses, with
// E-RPC-005, one that is not valid or not in the file named after its app.
func loadHeld(root *os.Root) (map[string]rpcschema.Schema, error) {
	apps, err := heldApps(root)
	if err != nil {
		return nil, err
	}
	held := map[string]rpcschema.Schema{}
	for _, app := range apps {
		rel := heldDir + "/" + app + ".json"
		data, err := root.ReadFile(filepath.FromSlash(rel))
		if err != nil {
			return nil, err
		}
		s, err := rpcschema.Parse(rel, data)
		if err != nil {
			return nil, err
		}
		if s.App != app {
			return nil, errs.At(rel+":1", "E-RPC-005", fmt.Sprintf("%s holds the snapshot of the app %q; its name must be <app>.json", rel, s.App),
				"run aicoded rpc add <app> again for a fresh copy in .aicoded/services/; never edit or rename a snapshot by hand")
		}
		held[app] = s
	}
	return held, nil
}

// servicesSection renders the services section of the permission list, marker comment included.
// Its calls and serves parts are left out when they are empty, and so are the require rules of a
// function without role= and the apps flag of one without apps=true.
func servicesSection(s manifest.Services) ([]byte, error) {
	body := &yaml.Node{Kind: yaml.MappingNode}
	if len(s.Calls) > 0 {
		calls := &yaml.Node{Kind: yaml.MappingNode}
		for _, app := range slices.Sorted(maps.Keys(s.Calls)) {
			calls.Content = append(calls.Content, quoted(app), list(s.Calls[app]))
		}
		body.Content = append(body.Content, plain("calls"), calls)
	}
	if len(s.Serves) > 0 {
		serves := &yaml.Node{Kind: yaml.MappingNode}
		for _, name := range slices.Sorted(maps.Keys(s.Serves)) {
			e := s.Serves[name]
			v := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{plain("callers"), list(e.Callers)}}
			if len(e.Require) > 0 {
				v.Content = append(v.Content, plain("require"), list(e.Require))
			}
			if e.Apps {
				v.Content = append(v.Content, plain("apps"), yes())
			}
			serves.Content = append(serves.Content, quoted(name), v)
		}
		body.Content = append(body.Content, plain("serves"), serves)
	}
	return encodeSection("services", servicesMarker, body)
}
