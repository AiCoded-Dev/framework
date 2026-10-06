// Package manifest loads and checks the slice of aicoded.yaml the CLI reads.
package manifest

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/mod/module"
	"gopkg.in/yaml.v3"

	"aicoded.dev/framework/cmd/aicoded/internal/yamlerr"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/lint"
	"aicoded.dev/framework/runnerproto/mailrules"
)

// FileName is the name of the permission list in an app's directory.
const FileName = "aicoded.yaml"

// Manifest is the slice of aicoded.yaml the CLI reads.
type Manifest struct {
	App      string            `yaml:"app"`
	Data     []Data            `yaml:"data"`
	Email    *Email            `yaml:"email"`
	Settings []string          `yaml:"settings"`
	Secrets  []string          `yaml:"secrets"`
	Modules  []string          `yaml:"modules"`
	Access   map[string]Access `yaml:"access"`
	Services Services          `yaml:"services"`
}

// Access is one entry of the access section aicoded generate writes.
type Access struct {
	Require []string `yaml:"require"`
	Guard   bool     `yaml:"guard"`
	Calls   []string `yaml:"calls"`
}

// Services is the services section aicoded generate writes: the functions of other apps this
// app calls, by app, and the functions it serves to other apps, by name.
type Services struct {
	Calls  map[string][]string `yaml:"calls"`
	Serves map[string]Serve    `yaml:"serves"`
}

// Serve says who may call one function the app serves: the calling apps, the role rules a
// viewer must meet, written as in the access section, and whether a calling app may call it with
// no viewer. A function without role rules takes no call on behalf of a viewer.
type Serve struct {
	Callers []string `yaml:"callers"`
	Require []string `yaml:"require,omitempty"`
	Apps    bool     `yaml:"apps,omitempty"`
}

// Data is one source of data the app reaches: sqldb, filestore:<name> or connector:<name>,
// with the classes of data it holds.
type Data struct {
	Source  string   `yaml:"source"`
	Classes []string `yaml:"classes"`
}

// Email is the address the app sends mail from and the domains it may send to.
type Email struct {
	From      string   `yaml:"from"`
	ToDomains []string `yaml:"to_domains"`
}

// SQLDB reports whether the app declares its SQL database.
func (m Manifest) SQLDB() bool {
	return slices.ContainsFunc(m.Data, func(d Data) bool { return d.Source == "sqldb" })
}

// Stores returns the names of the file stores the app declares, sorted.
func (m Manifest) Stores() []string {
	var out []string
	for _, d := range m.Data {
		if name, ok := strings.CutPrefix(d.Source, "filestore:"); ok {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// Roles returns every role the access and services sections name, sorted, without "*".
func (m Manifest) Roles() []string {
	set := map[string]bool{}
	add := func(rules []string) {
		for _, rule := range rules {
			for r := range strings.SplitSeq(rule, "|") {
				if r != "*" && r != "" {
					set[r] = true
				}
			}
		}
	}
	for _, a := range m.Access {
		add(a.Require)
	}
	for _, s := range m.Services.Serves {
		add(s.Require)
	}
	return slices.Sorted(maps.Keys(set))
}

var (
	appName    = regexp.MustCompile(`^[a-z][a-z0-9-]{0,61}[a-z0-9]$`)
	valueName  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	sourceName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	className  = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
)

// ValidApp reports whether name is a valid app name: 2-63 lowercase letters, digits and dashes,
// starting with a letter and ending with a letter or digit.
func ValidApp(name string) bool {
	return appName.MatchString(name)
}

// Load reads and checks the manifest at path. Sections this package does not know are ignored.
func Load(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Manifest{}, errs.New("E-MAN-006", path+" does not exist", "run aicoded in the app's directory, next to aicoded.yaml")
	}
	if err != nil {
		return Manifest{}, err
	}
	doc, err := Document(path, data)
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if len(doc.Content) > 0 {
		if err := doc.Decode(&m); err != nil {
			return Manifest{}, errs.At(errPos(path, err), "E-MAN-003", "aicoded.yaml has an unexpected shape: "+err.Error(),
				"compare it with the example in the docs")
		}
	}
	at := func(key string, item int) string { return fmt.Sprintf("%s:%d", path, line(&doc, key, item)) }
	if !ValidApp(m.App) {
		return Manifest{}, errs.At(at("app", -1), "E-MAN-004", fmt.Sprintf("app name %q is not valid", m.App),
			"use 2-63 lowercase letters, digits and dashes, starting with a letter")
	}
	if err := checkNames(m.Settings, "settings", at); err != nil {
		return Manifest{}, err
	}
	if err := checkNames(m.Secrets, "secrets", at); err != nil {
		return Manifest{}, err
	}
	if err := checkData(m.Data, at); err != nil {
		return Manifest{}, err
	}
	if err := checkEmail(m.Email, at); err != nil {
		return Manifest{}, err
	}
	if err := checkModules(m.Modules, at); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// Document returns the YAML document of the permission list data read from path, or an empty
// node when data holds none. It refuses data that is not valid YAML or holds more than one
// document with E-MAN-003 at path:line.
func Document(path string, data []byte) (yaml.Node, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc, next yaml.Node
	if err := dec.Decode(&doc); errors.Is(err, io.EOF) {
		return yaml.Node{}, nil
	} else if err != nil {
		return yaml.Node{}, errs.At(errPos(path, err), "E-MAN-003", "aicoded.yaml is not valid YAML: "+err.Error(), "fix the YAML syntax")
	}
	if err := dec.Decode(&next); !errors.Is(err, io.EOF) {
		pos := fmt.Sprintf("%s:%d", path, next.Line)
		if err != nil {
			pos = errPos(path, err)
		}
		return yaml.Node{}, errs.At(pos, "E-MAN-003", "aicoded.yaml has more than one YAML document",
			"keep one YAML document: remove the --- line and everything below it")
	}
	return doc, nil
}

func errPos(path string, err error) string {
	line, ok := yamlerr.Line(err)
	if !ok {
		line = 1
	}
	return fmt.Sprintf("%s:%d", path, line)
}

func checkNames(names []string, key string, at func(string, int) string) error {
	seen := map[string]bool{}
	for i, n := range names {
		if !valueName.MatchString(n) {
			return errs.At(at(key, i), "E-MAN-005", fmt.Sprintf("%s name %q is not valid", key, n),
				"use lowercase letters, digits and underscores, starting with a letter")
		}
		if seen[n] {
			return errs.At(at(key, i), "E-MAN-005", fmt.Sprintf("%s name %q is listed twice", key, n), "remove the duplicate")
		}
		seen[n] = true
	}
	return nil
}

func checkData(ds []Data, at func(string, int) string) error {
	seen := map[string]bool{}
	for i, d := range ds {
		kind, name, named := strings.Cut(d.Source, ":")
		valid := d.Source == "sqldb" || named && (kind == "filestore" || kind == "connector") && sourceName.MatchString(name)
		if !valid {
			return errs.At(at("data", i), "E-MAN-007", fmt.Sprintf("data source %q is not valid", d.Source),
				"use `sqldb`, `filestore:<name>` or `connector:<name>` with a lower-case name")
		}
		if seen[d.Source] {
			return errs.At(at("data", i), "E-MAN-007", fmt.Sprintf("data source %q is listed twice", d.Source), "remove the duplicate")
		}
		seen[d.Source] = true
		if len(d.Classes) == 0 {
			return errs.At(at("data", i), "E-MAN-008", fmt.Sprintf("data source %q has no data classes", d.Source),
				"list the classes of data it holds, such as classes: [internal]")
		}
		for _, c := range d.Classes {
			if !className.MatchString(c) {
				return errs.At(at("data", i), "E-MAN-008", fmt.Sprintf("data class %q of %s is not valid", c, d.Source),
					"use lower-case letters, digits and dashes, such as internal")
			}
		}
	}
	return nil
}

func checkEmail(e *Email, at func(string, int) string) error {
	if e == nil {
		return nil
	}
	fail := func(msg string) error {
		return errs.At(at("email", -1), "E-MAN-009", msg,
			"write from: name@your.domain and to_domains: [your.domain], lower-case ASCII, no wildcards")
	}
	if mailrules.Address(e.From) != nil || e.From != strings.ToLower(e.From) {
		return fail(fmt.Sprintf("email.from %q is not a plain lower-case address", e.From))
	}
	if len(e.ToDomains) == 0 {
		return fail("email.to_domains lists no domain")
	}
	seen := map[string]bool{}
	for _, d := range e.ToDomains {
		if !mailrules.ValidDomain(d) || seen[d] {
			return fail(fmt.Sprintf("email.to_domains entry %q is not a lower-case domain, or is listed twice", d))
		}
		seen[d] = true
	}
	return nil
}

func checkModules(paths []string, at func(string, int) string) error {
	seen := map[string]bool{}
	for i, p := range paths {
		if err := module.CheckPath(p); err != nil {
			return errs.At(at("modules", i), "E-MAN-013", fmt.Sprintf("modules entry %q is not a module path", p),
				"write the module path as go.mod requires it, such as golang.org/x/text, with no version")
		}
		if p == lint.Framework || strings.HasPrefix(p, lint.Framework+"/") {
			return errs.At(at("modules", i), "E-MAN-013", fmt.Sprintf("modules entry %q is the framework, which needs no declaration", p),
				"remove it: modules: lists only third-party modules")
		}
		if seen[p] {
			return errs.At(at("modules", i), "E-MAN-013", fmt.Sprintf("modules entry %q is listed twice", p), "remove the duplicate")
		}
		seen[p] = true
	}
	return nil
}

// line returns the line of item under key in the top-level mapping, or of the key itself
// when item < 0, falling back to the mapping itself.
func line(doc *yaml.Node, key string, item int) int {
	if len(doc.Content) == 0 {
		return 1
	}
	m := doc.Content[0]
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value != key {
			continue
		}
		if v := m.Content[i+1]; item >= 0 && item < len(v.Content) {
			return v.Content[item].Line
		}
		return m.Content[i].Line
	}
	return m.Line
}
