// Package rpcadd lets an app call the functions of another app of its workspace.
package rpcadd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"aicoded.dev/framework/cmd/aicoded/internal/generate"
	"aicoded.dev/framework/cmd/aicoded/internal/manifest"
	"aicoded.dev/framework/cmd/aicoded/internal/rpcgen"
	"aicoded.dev/framework/cmd/aicoded/internal/rpcschema"
	"aicoded.dev/framework/cmd/aicoded/internal/workspace"
	"aicoded.dev/framework/internal/errs"
)

// Add lets the app in dir call the app named app. It finds that app among the apps under the
// parent folder of dir, generates it with the other apps there as its workspace, copies its
// snapshot .aicoded/rpc.json into dir as .aicoded/services/<app>.json, and generates the app in
// dir, which writes the client and the services section. The snapshot dir held before does not
// stop the called app's generation, since Add replaces it. It refuses, with E-RPC-006, what
// rpcgen.AddedPackageName refuses, and, with E-RPC-007, an app that is not there, the app in dir
// itself, and an app that serves no function. It writes nothing into dir when it refuses.
func Add(dir, app string, out io.Writer) error {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if _, err := manifest.Load(filepath.Join(dir, manifest.FileName)); err != nil {
		return err
	}
	held, err := generate.HeldApps(dir)
	if err != nil {
		return err
	}
	pkg, err := rpcgen.AddedPackageName(app, held)
	if err != nil {
		return err
	}
	callee, err := workspace.Find(filepath.Dir(dir), app)
	if err != nil {
		return err
	}
	same, err := sameFolder(callee, dir)
	if err != nil {
		return err
	}
	if same {
		return errs.New("E-RPC-007", "the app "+app+" in "+dir+" cannot call itself",
			"call the functions of this app's rpc/ package directly in Go, and name another app in aicoded rpc add")
	}
	if st, err := os.Stat(filepath.Join(callee, "rpc")); err != nil || !st.IsDir() {
		return servesNothing(app, callee, "has no rpc/ package")
	}
	others, err := otherApps(filepath.Dir(dir), dir)
	if err != nil {
		return err
	}
	if _, err := generate.Run(callee, generate.Options{Workspace: others}); err != nil {
		return err
	}
	s, err := snapshot(callee)
	if err != nil {
		return err
	}
	if len(s.Methods) == 0 {
		return servesNothing(app, callee, "has no exported function in its rpc/ package")
	}
	if err := generate.WriteFile(dir, ".aicoded/services/"+app+".json", s.Marshal()); err != nil {
		return err
	}
	if err := generate.Generate(dir); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "aicoded rpc add: call the functions of %s through the package in services/%s\n", app, pkg)
	return err
}

// servesNothing refuses, with E-RPC-007, the app in dir, which has no function to call for the
// reason why.
func servesNothing(app, dir, why string) error {
	return errs.New("E-RPC-007", fmt.Sprintf("the app %s in %s %s, so it serves no functions", app, dir, why),
		"add the functions to call to the rpc/ package of "+app+", each with an //ssr:access line that names the calling app")
}

// otherApps returns, by name, the apps under root other than the app in dir. It fails with
// E-DEV-010 when two of them have one name.
func otherApps(root, dir string) (map[string]string, error) {
	dirs, err := workspace.Discover(root)
	if err != nil {
		return nil, err
	}
	apps := map[string]string{}
	for _, d := range dirs {
		same, err := sameFolder(d, dir)
		if err != nil {
			return nil, err
		}
		if same {
			continue
		}
		m, err := manifest.Load(filepath.Join(d, manifest.FileName))
		if err != nil {
			return nil, err
		}
		if other, ok := apps[m.App]; ok {
			return nil, workspace.SameName(m.App, other, d)
		}
		apps[m.App] = d
	}
	return apps, nil
}

// sameFolder reports whether the paths a and b name the same folder.
func sameFolder(a, b string) (bool, error) {
	sa, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	sb, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	return os.SameFile(sa, sb), nil
}

// snapshot reads the snapshot .aicoded/rpc.json of the app in dir, never following a symbolic
// link out of dir.
func snapshot(dir string) (rpcschema.Schema, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return rpcschema.Schema{}, err
	}
	defer root.Close()
	data, err := root.ReadFile(filepath.Join(".aicoded", "rpc.json"))
	if err != nil {
		return rpcschema.Schema{}, err
	}
	return rpcschema.Parse(filepath.Join(dir, ".aicoded", "rpc.json"), data)
}
