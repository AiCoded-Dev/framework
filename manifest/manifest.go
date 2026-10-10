package manifest

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"maps"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
	"golang.org/x/mod/module"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/yamldoc"
	"aicoded.dev/framework/runnerproto/mailrules"
)

// FileName is the name of the permission list in an app's directory.
const FileName = "aicoded.yaml"

// framework is the module path of the framework.
const framework = "aicoded.dev/framework"

// Manifest is the permission list, aicoded.yaml. A field left out or empty is not stated.
type Manifest struct {
	App       string            `yaml:"app"`
	Class     string            `yaml:"class"`
	Owner     string            `yaml:"owner"`
	Audience  Audience          `yaml:"audience"`
	Data      []Data            `yaml:"data"`
	Access    map[string]Access `yaml:"access"`
	Services  Services          `yaml:"services"`
	Egress    []string          `yaml:"egress"`
	Email     *Email            `yaml:"email"`
	Schedule  []Job             `yaml:"schedule"`
	Settings  []string          `yaml:"settings"`
	Secrets   []string          `yaml:"secrets"`
	Modules   []string          `yaml:"modules"`
	Size      string            `yaml:"size"`
	Resources string            `yaml:"resources"`
	TTL       string            `yaml:"ttl"`
}

// Audience is who may use the app: people of the company, as group:<name> or everyone, and
// outside people, as idp:<name> or magic-link.
type Audience struct {
	Internal []string `yaml:"internal"`
	External []string `yaml:"external"`
}

// Job is one scheduled job: its name and the cron expression, in UTC, of when it runs.
type Job struct {
	Cron string `yaml:"cron"`
	Name string `yaml:"job"`
}

// Access is one entry of the access section aicoded generate writes: the role rules a viewer
// must meet, whether a Guard on the page's path decides who sees each record, whether everyone
// the rules admit may see every record, and the page calls. Guard and Shared are never both
// set.
type Access struct {
	Require []string `yaml:"require"`
	Guard   bool     `yaml:"guard"`
	Shared  bool     `yaml:"shared"`
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
	return m.sources("filestore:")
}

// Connectors returns the names of the connectors the app declares, sorted.
func (m Manifest) Connectors() []string {
	return m.sources("connector:")
}

// sources returns the names after prefix of the data sources that start with it, sorted.
func (m Manifest) sources(prefix string) []string {
	var out []string
	for _, d := range m.Data {
		if name, ok := strings.CutPrefix(d.Source, prefix); ok {
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
	appName   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,61}[a-z0-9]$`)
	valueName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	// lowerName is the shape of the names of data sources, data classes, app types, identity
	// providers and jobs.
	lowerName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
)

// ValidApp reports whether name is a valid app name: 2-63 lowercase letters, digits and dashes,
// starting with a letter and ending with a letter or digit.
func ValidApp(name string) bool {
	return appName.MatchString(name)
}

// Load reads and checks the manifest at path, as Parse does.
func Load(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Manifest{}, errs.New("E-MAN-006", path+" does not exist", "run aicoded in the app's directory, next to aicoded.yaml")
	}
	if err != nil {
		return Manifest{}, err
	}
	return Parse(path, data)
}

// Parse checks the manifest data and returns it. Path is used only in the positions of
// problems. It reports every problem it finds as one coded error each: its error unwraps, with
// Unwrap() []error, to them, sorted by line. Data that is not one YAML document of the expected
// shape, or that uses an anchor, an alias or a merge key, is not checked any further.
func Parse(path string, data []byte) (Manifest, error) {
	doc, err := yamldoc.Document(path, data)
	if err != nil {
		return Manifest{}, err
	}
	c := checker{path: path, field: -1}
	var m Manifest
	if len(doc.Content) > 0 {
		c.root = doc.Content[0]
		decodeErr := doc.Decode(&m)
		if decodeErr != nil {
			c.add(yamldoc.LineOr1(decodeErr), "E-MAN-003", "aicoded.yaml has an unexpected shape: "+decodeErr.Error(),
				"compare it with the example in the docs")
		}
		c.unknownKeys()
		if decodeErr != nil {
			return Manifest{}, c.err()
		}
	}
	c.check(m)
	if err := c.err(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// fieldKeys are the top-level keys, in the order of Manifest's fields.
var fieldKeys = keys[Manifest]()

// keys returns the YAML keys of the fields of the struct type T, in order.
func keys[T any]() []string {
	t := reflect.TypeFor[T]()
	out := make([]string, t.NumField())
	for i := range out {
		out[i], _, _ = strings.Cut(t.Field(i).Tag.Get("yaml"), ",")
	}
	return out
}

// checker collects the problems of one permission list.
type checker struct {
	path string
	root *yaml.Node // the document's top-level node; nil when the document is empty
	// field is the index in fieldKeys of the field being checked, which orders the problems
	// found on one line; -1 for problems of the whole file.
	field    int
	problems []problem
}

type problem struct {
	line, field int
	err         *errs.Error
}

func (c *checker) add(line int, code, msg, fix string) {
	pos := fmt.Sprintf("%s:%d", c.path, line)
	c.problems = append(c.problems, problem{line, c.field, errs.At(pos, code, msg, fix)})
}

// err returns the problems sorted by line, then field, joined, or nil when there are none.
func (c *checker) err() error {
	slices.SortStableFunc(c.problems, func(a, b problem) int {
		return cmp.Or(cmp.Compare(a.line, b.line), cmp.Compare(a.field, b.field))
	})
	out := make([]error, len(c.problems))
	for i, p := range c.problems {
		out[i] = p.err
	}
	return errors.Join(out...)
}

// section starts the checks of the top-level key and returns its value node, or nil.
func (c *checker) section(key string) *yaml.Node {
	c.field = slices.Index(fieldKeys, key)
	return value(c.root, key)
}

// keyLine returns the line of the top-level key, of the document when it has no such key, or 1.
func (c *checker) keyLine(key string) int {
	if c.root == nil {
		return 1
	}
	for i := 0; i+1 < len(c.root.Content); i += 2 {
		if c.root.Content[i].Value == key {
			return c.root.Content[i].Line
		}
	}
	return c.root.Line
}

// value returns the value of key in the mapping node m, or nil.
func value(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// item returns the i-th node of the sequence node n, or n when it has none.
func item(n *yaml.Node, i int) *yaml.Node {
	if n != nil && i < len(n.Content) {
		return n.Content[i]
	}
	return n
}

// line returns the line of n, or 1 for nil.
func line(n *yaml.Node) int {
	if n == nil {
		return 1
	}
	return n.Line
}

// unknownKeys reports every key of a mapping with fixed keys that is not one of them.
func (c *checker) unknownKeys() {
	const generated = "remove it: aicoded generate writes this section from the code, so never edit it"
	c.keys(c.root, fieldKeys, "the top level", "")
	c.keys(c.section("audience"), keys[Audience](), "audience", "")
	for _, d := range seq(c.section("data")) {
		c.keys(d, keys[Data](), "data", "")
	}
	for path, a := range pairs(c.section("access")) {
		c.keys(a, keys[Access](), fmt.Sprintf("access %q", path), generated)
	}
	services := c.section("services")
	c.keys(services, keys[Services](), "services", generated)
	for name, s := range pairs(value(services, "serves")) {
		c.keys(s, keys[Serve](), fmt.Sprintf("services.serves %q", name), generated)
	}
	c.keys(c.section("email"), keys[Email](), "email", "")
	for _, j := range seq(c.section("schedule")) {
		c.keys(j, keys[Job](), "schedule", "")
	}
	c.field = -1
}

// keys reports the keys of the mapping node m that are not in known. Fix replaces the default
// fix, which lists the known keys.
func (c *checker) keys(m *yaml.Node, known []string, place, fix string) {
	if m == nil || m.Kind != yaml.MappingNode {
		return
	}
	if fix == "" {
		fix = "remove it, or correct its name: the keys there are " + strings.Join(known, ", ")
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if k := m.Content[i]; k.Kind == yaml.ScalarNode && !slices.Contains(known, k.Value) {
			c.add(k.Line, "E-MAN-014", fmt.Sprintf("unknown key %q in %s", k.Value, place), fix)
		}
	}
}

// seq returns the items of the sequence node n, or nil.
func seq(n *yaml.Node) []*yaml.Node {
	if n == nil || n.Kind != yaml.SequenceNode {
		return nil
	}
	return n.Content
}

// pairs yields the scalar keys and the values of the mapping node n.
func pairs(n *yaml.Node) iter.Seq2[string, *yaml.Node] {
	return func(yield func(string, *yaml.Node) bool) {
		if n == nil || n.Kind != yaml.MappingNode {
			return
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			if !yield(n.Content[i].Value, n.Content[i+1]) {
				return
			}
		}
	}
}

// check checks every field of m, in order.
func (c *checker) check(m Manifest) {
	c.section("app")
	if !ValidApp(m.App) {
		c.add(c.keyLine("app"), "E-MAN-004", fmt.Sprintf("app name %q is not valid", m.App),
			"use 2-63 lowercase letters, digits and dashes, starting with a letter")
	}
	c.checkClass(m.Class, c.section("class"))
	c.checkOwner(m.Owner, c.section("owner"))
	c.checkAudience(m.Audience, c.section("audience"))
	c.checkData(m.Data, c.section("data"))
	c.checkAccess(m.Access, c.section("access"))
	c.checkEgress(m.Egress, c.section("egress"))
	c.section("email")
	c.checkEmail(m.Email)
	c.checkSchedule(m.Schedule, c.section("schedule"))
	c.checkNames(m.Settings, "settings", c.section("settings"))
	c.checkNames(m.Secrets, "secrets", c.section("secrets"))
	c.checkModules(m.Modules, c.section("modules"))
	c.checkChoice(m.Size, "size", c.section("size"), "E-MAN-020", "S", "M", "L")
	c.checkChoice(m.Resources, "resources", c.section("resources"), "E-MAN-021", "small", "medium", "large")
	c.checkTTL(m.TTL, c.section("ttl"))
}

func (c *checker) checkNames(names []string, key string, n *yaml.Node) {
	seen := map[string]bool{}
	for i, name := range names {
		switch {
		case !valueName.MatchString(name):
			c.add(line(item(n, i)), "E-MAN-005", fmt.Sprintf("%s name %q is not valid", key, name),
				"use lowercase letters, digits and underscores, starting with a letter")
		case seen[name]:
			c.add(line(item(n, i)), "E-MAN-005", fmt.Sprintf("%s name %q is listed twice", key, name), "remove the duplicate")
		}
		seen[name] = true
	}
}

// checkAccess refuses an access entry that is both guarded and shared, which aicoded generate
// never writes. The problem is at the entry's path.
func (c *checker) checkAccess(access map[string]Access, n *yaml.Node) {
	if n == nil || n.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		path := n.Content[i]
		if a := access[path.Value]; a.Guard && a.Shared {
			c.add(path.Line, "E-MAN-023", fmt.Sprintf("access %q is both guarded and shared", path.Value),
				"run aicoded generate and do not edit the access section by hand")
		}
	}
}

func (c *checker) checkData(ds []Data, n *yaml.Node) {
	seen := map[string]bool{}
	for i, d := range ds {
		at := line(item(n, i))
		kind, name, named := strings.Cut(d.Source, ":")
		valid := d.Source == "sqldb" || named && (kind == "filestore" || kind == "connector") && lowerName.MatchString(name)
		switch {
		case !valid:
			c.add(at, "E-MAN-007", fmt.Sprintf("data source %q is not valid", d.Source),
				"use `sqldb`, `filestore:<name>` or `connector:<name>` with a lower-case name")
		case seen[d.Source]:
			c.add(at, "E-MAN-007", fmt.Sprintf("data source %q is listed twice", d.Source), "remove the duplicate")
		}
		seen[d.Source] = true
		if len(d.Classes) == 0 {
			c.add(at, "E-MAN-008", fmt.Sprintf("data source %q has no data classes", d.Source),
				"list the classes of data it holds, such as classes: [internal]")
		}
		for _, class := range d.Classes {
			if !lowerName.MatchString(class) {
				c.add(at, "E-MAN-008", fmt.Sprintf("data class %q of %s is not valid", class, d.Source),
					"use lower-case letters, digits and dashes, such as internal")
			}
		}
	}
}

func (c *checker) checkEmail(e *Email) {
	if e == nil {
		return
	}
	fail := func(msg string) {
		c.add(c.keyLine("email"), "E-MAN-009", msg,
			"write from: name@your.domain and to_domains: [your.domain], lower-case ASCII, no wildcards")
	}
	if mailrules.Address(e.From) != nil || e.From != strings.ToLower(e.From) {
		fail(fmt.Sprintf("email.from %q is not a plain lower-case address", e.From))
	}
	if len(e.ToDomains) == 0 {
		fail("email.to_domains lists no domain")
	}
	seen := map[string]bool{}
	for _, d := range e.ToDomains {
		if !mailrules.ValidDomain(d) || seen[d] {
			fail(fmt.Sprintf("email.to_domains entry %q is not a lower-case domain, or is listed twice", d))
		}
		seen[d] = true
	}
}

func (c *checker) checkModules(paths []string, n *yaml.Node) {
	seen := map[string]bool{}
	for i, p := range paths {
		at := line(item(n, i))
		switch {
		case module.CheckPath(p) != nil:
			c.add(at, "E-MAN-013", fmt.Sprintf("modules entry %q is not a module path", p),
				"write the module path as go.mod requires it, such as golang.org/x/text, with no version")
		case p == framework || strings.HasPrefix(p, framework+"/"):
			c.add(at, "E-MAN-013", fmt.Sprintf("modules entry %q is the framework, which needs no declaration", p),
				"remove it: modules: lists only third-party modules")
		case seen[p]:
			c.add(at, "E-MAN-013", fmt.Sprintf("modules entry %q is listed twice", p), "remove the duplicate")
		}
		seen[p] = true
	}
}
