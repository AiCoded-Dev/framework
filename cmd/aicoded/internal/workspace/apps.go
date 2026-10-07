package workspace

import (
	"fmt"
	"path/filepath"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/manifest"
)

// App is one app of a workspace: the name on the app: line of its permission list, and its
// folder.
type App struct{ Name, Dir string }

// Apps returns the apps under root in folder order. It fails with E-MAN-006 when there is
// none, with E-DEV-010 when two have one name, and with the error of a permission list that does
// not load.
func Apps(root string) ([]App, error) {
	dirs, err := Discover(root)
	if err != nil {
		return nil, err
	}
	if len(dirs) == 0 {
		return nil, errs.New("E-MAN-006", "no "+manifest.FileName+" in "+root+" or the folders below it",
			"run the command in an app's folder, next to aicoded.yaml, or in a folder above apps")
	}
	apps := make([]App, len(dirs))
	names := map[string]string{}
	for i, dir := range dirs {
		m, err := manifest.Load(filepath.Join(dir, manifest.FileName))
		if err != nil {
			return nil, err
		}
		if other, ok := names[m.App]; ok {
			return nil, SameName(m.App, other, dir)
		}
		names[m.App] = dir
		apps[i] = App{Name: m.App, Dir: dir}
	}
	return apps, nil
}

// SameName is E-DEV-010 for the apps in the folders a and b, which are both named name.
func SameName(name, a, b string) error {
	return errs.New("E-DEV-010", fmt.Sprintf("the apps in %s and %s are both named %s", a, b, name),
		"give each app its own name in the app: line of its aicoded.yaml")
}

// Names maps the name of each app to its folder.
func Names(apps []App) map[string]string {
	names := make(map[string]string, len(apps))
	for _, a := range apps {
		names[a.Name] = a.Dir
	}
	return names
}

// Select returns the app of apps named name, or every app when name is "". It fails with
// E-DEV-014 when no app has the name.
func Select(apps []App, name string) ([]App, error) {
	if name == "" {
		return apps, nil
	}
	for _, a := range apps {
		if a.Name == name {
			return []App{a}, nil
		}
	}
	return nil, UnknownApp(name)
}

// UnknownApp is E-DEV-014 for name, which no app of the workspace has; name may be any text.
func UnknownApp(name string) error {
	if len(name) > 64 {
		name = name[:64] + "…"
	}
	return errs.New("E-DEV-014", fmt.Sprintf("no app named %q in this workspace", name),
		"name an app from the app: line of an aicoded.yaml in this workspace")
}
