// Package devconfig reads the developer's own settings for aicoded dev: ports, the apps'
// environment, personas, the local MySQL server and local values of settings and secrets. The
// file holds secrets, so only its owner may read it.
package devconfig

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/yamldoc"
)

// DefaultPort is the dev gateway's port when the workspace sets none.
const DefaultPort = 8080

type config struct {
	Workspaces map[string]Workspace `yaml:"workspaces"`
}

// Workspace holds the settings for one directory tree of apps.
type Workspace struct {
	Port int `yaml:"port"`
	// Env is the environment the runner tells the apps they run in.
	Env Env `yaml:"env"`
	// MySQL is the DSN of an admin user of a MySQL 8 server on this machine; it creates the
	// apps' databases.
	MySQL    string               `yaml:"mysql"`
	Personas []Persona            `yaml:"personas"`
	Apps     map[string]AppValues `yaml:"apps"`
}

// Env is an environment a workspace's apps can run in.
type Env string

const (
	// EnvDev is aicoded dev's own environment, the default.
	EnvDev Env = "dev"
	// EnvPreview runs the apps as a preview, so that the framework behaves as it does outside
	// aicoded dev.
	EnvPreview Env = "preview"
)

// Valid reports whether e is EnvDev or EnvPreview.
func (e Env) Valid() bool { return e == EnvDev || e == EnvPreview }

// UnmarshalYAML accepts only a valid Env.
func (e *Env) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode || !Env(n.Value).Valid() {
		return envError{line: n.Line}
	}
	*e = Env(n.Value)
	return nil
}

// envError is an env value at line of dev.yaml that is not a valid Env.
type envError struct{ line int }

func (e envError) Error() string { return fmt.Sprintf("line %d: env is not dev or preview", e.line) }

// Persona is a test viewer the dev gateway can sign in as.
type Persona struct {
	Name   string   `yaml:"name"`
	Groups []string `yaml:"groups"`
	Roles  []string `yaml:"roles"`
}

// AppValues are local values for the settings and secrets an app declares.
type AppValues struct {
	Settings map[string]string `yaml:"settings"`
	Secrets  map[string]string `yaml:"secrets"`
}

// DefaultPath returns the location of dev.yaml in the user's configuration directory.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "aicoded", "dev.yaml"), nil
}

// Load returns the settings for the workspace rooted at root. A missing file gives defaults.
// Personas stay empty when the workspace names none; the caller picks the defaults.
func Load(path, root string) (Workspace, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return withDefaults(Workspace{}), nil
	}
	if err != nil {
		return Workspace{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return Workspace{}, err
	}
	if perm := st.Mode().Perm(); perm&0o077 != 0 {
		return Workspace{}, errs.New("E-DEV-001", fmt.Sprintf("%s can be read by other users (mode %04o)", path, perm),
			"run: chmod 600 "+path)
	}
	var c config
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		var env envError
		if errors.As(err, &env) {
			return Workspace{}, errs.At(fmt.Sprintf("%s:%d", path, env.line), "E-DEV-020", "env in dev.yaml must be dev or preview",
				"set env: to dev or preview, or remove the line to run the apps as dev")
		}
		return Workspace{}, invalid(path, err)
	}
	return withDefaults(c.Workspaces[root]), nil
}

// invalid reports a decoding error by file and line only, never with yaml's message, which can
// quote part of a secret value.
func invalid(path string, err error) error {
	if line, ok := yamldoc.Line(err); ok {
		return errs.At(fmt.Sprintf("%s:%d", path, line), "E-DEV-002", "dev.yaml is not valid",
			"fix the file at this line; its format is in the docs")
	}
	return errs.New("E-DEV-002", path+" is not valid", "fix the file; its format is in the docs")
}

// withDefaults sets the port to DefaultPort and the environment to EnvDev when w has none.
func withDefaults(w Workspace) Workspace {
	if w.Port == 0 {
		w.Port = DefaultPort
	}
	if w.Env == "" {
		w.Env = EnvDev
	}
	return w
}
