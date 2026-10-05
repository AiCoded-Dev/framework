package generate

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/template"
	"aicoded.dev/framework/internal/errs"
)

// Route is a folder under pages/ with an index.html. Path is its URL pattern, such as
// /notes/n_id, and Dir its absolute folder.
type Route struct {
	Path     string
	Dir      string
	Template *template.Template
	// live is set by discover when the page has live variables.
	live *livePage
}

// reservedFolders are folder names the pages handler serves itself, at any depth.
var reservedFolders = map[string]bool{"__ws": true, "_aicoded": true}

// discover finds the routes under the app's pages/ folder, sorted by path. It refuses reserved
// folder names, folders Go tools skip, a page with no access rule on its path, an access rule
// that adds nothing to a rule above it, a Guard method the route does not declare, a default
// page that is not a page below its layout, two parameter folders in one folder, folder names
// Go code cannot use, two pages with one route key, live values the page cannot show or write
// and <ssr:assets/> or <ssr:content/> inside a condition or loop. It reports every problem of
// the run together as Diagnostics.
func discover(app App, image func(routeDir, src string) (string, error)) ([]*Route, error) {
	pages := filepath.Join(app.Dir, "pages")
	var (
		diags  Diagnostics
		routes = map[string]*Route{}
		broken = map[string]bool{} // routes whose template did not parse
	)
	err := filepath.WalkDir(pages, func(dir string, d fs.DirEntry, err error) error {
		if err != nil {
			if dir == pages && errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if !d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(pages, dir)
		if err != nil {
			return err
		}
		routePath := path.Join("/", filepath.ToSlash(rel))
		if dir != pages {
			switch name := d.Name(); {
			case name == assetsDir && filepath.Dir(dir) == pages:
				return filepath.SkipDir
			case reservedFolders[name] || name == assetsDir:
				diags = append(diags, diag(templatePos(routePath), "E-GEN-016", "%s is a reserved folder name", name))
				return filepath.SkipDir
			case skippedByGo(name):
				pos := folder(routePath)
				if _, err := os.Lstat(filepath.Join(dir, "index.html")); err == nil {
					pos = templatePos(routePath)
				}
				diags = append(diags, diag(pos, "E-GEN-043", "Go tools skip a folder named %q, so the security checks would never see its code", name))
				return filepath.SkipDir
			}
		}
		tpl, err := template.Parse(filepath.Join(dir, "index.html"), templateFile(routePath),
			func(src string) (string, error) { return image(dir, src) })
		var e *errs.Error
		switch {
		case errors.As(err, &e):
			diags = append(diags, e)
			broken[routePath] = true
		case err != nil:
			return err
		case tpl != nil:
			routes[routePath] = &Route{Path: routePath, Dir: dir, Template: tpl}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	found := maps.Clone(broken)
	for p := range routes {
		found[p] = true
	}
	sorted := slices.SortedFunc(maps.Values(routes), func(a, b *Route) int { return strings.Compare(a.Path, b.Path) })
	for _, r := range sorted {
		if open(r.Path, routes, broken) {
			diags = append(diags, diag(templatePos(r.Path), "E-GEN-030", "%s has no access rule on its path", r.Path))
		}
		if e := addsNothing(r, routes); e != nil {
			diags = append(diags, e)
		}
		if e := checkDefault(r, found); e != nil {
			diags = append(diags, e)
		}
		if a := r.Template.Access(); a == nil || !a.Guard {
			guards, err := guardMethods(r)
			if err != nil {
				return nil, err
			}
			for _, pos := range guards {
				diags = append(diags, diag(pos, "E-GEN-032", `Guard is never called: %s has no <ssr:access> with guard="true"`, r.Path))
			}
		}
		diags = append(diags, checkLive(r, routes)...)
		diags = append(diags, placement(r)...)
	}
	diags = append(diags, paramConflicts(found)...)
	diags = append(diags, badFolders(found)...)
	diags = append(diags, keyConflicts(found)...)
	if err := diags.Err(); err != nil {
		return nil, err
	}
	return sorted, nil
}

// skippedByGo reports whether the go command leaves out a folder named name when it expands
// ./..., as vet, lint and tests do.
func skippedByGo(name string) bool {
	return strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") || name == "testdata"
}

// checkDefault refuses a <ssr:content default="…"/> that names no page below the route. The
// default is joined to the route's path, as the pages handler joins it to the URL, and every
// folder it names must have a fixed name: a parameter folder's value is not known here.
func checkDefault(r *Route, found map[string]bool) *errs.Error {
	c := r.Template.GetContentNode()
	if c == nil || c.Default == "" {
		return nil
	}
	pos := fmt.Sprintf("%s:%d", templateFile(r.Path), c.Line)
	target := path.Join(r.Path, c.Default)
	rel, below := strings.CutPrefix(target, strings.TrimSuffix(r.Path, "/")+"/")
	if !below || rel == "" {
		return diag(pos, "E-GEN-049", "<ssr:content default=%q> names %s, which is not below %s", c.Default, target, r.Path)
	}
	for _, seg := range segments(rel) {
		if isParam(seg) {
			return diag(pos, "E-GEN-049", "<ssr:content default=%q> names the parameter folder %s, whose value is not known here; "+
				"name the page in DefaultRoute instead", c.Default, seg)
		}
	}
	if !found[target] {
		return diag(pos, "E-GEN-049", "<ssr:content default=%q> names %s, where there is no page", c.Default, target)
	}
	return nil
}

// open reports whether no route on the path to p has an access rule. A template that did not
// parse has an unknown rule, so a path through it is not reported.
func open(p string, routes map[string]*Route, broken map[string]bool) bool {
	segs := segments(p)
	for i := 0; i <= len(segs); i++ {
		q := "/" + strings.Join(segs[:i], "/")
		if broken[q] || routes[q] != nil && routes[q].Template.Access() != nil {
			return false
		}
	}
	return true
}

// addsNothing refuses a route's own access rule that adds nothing to a rule above it on the
// path, since every rule on the path applies: a list of two or more different roles that
// includes every role of that rule, as a list admits anyone with one of its roles, or "*"
// without a guard below a rule that is not "*". It reports the nearest such rule above.
func addsNothing(r *Route, routes map[string]*Route) *errs.Error {
	c := r.Template.Access()
	if c == nil {
		return nil
	}
	star := c.Roles[0] == "*"
	if star && c.Guard || !star && len(slices.Compact(slices.Sorted(slices.Values(c.Roles)))) < 2 {
		return nil
	}
	segs := segments(r.Path)
	for i := len(segs) - 1; i >= 0; i-- {
		q := "/" + strings.Join(segs[:i], "/")
		if routes[q] == nil || routes[q].Template.Access() == nil {
			continue
		}
		p := routes[q].Template.Access()
		if star && p.Roles[0] == "*" || !star && !includes(c.Roles, p.Roles) {
			continue
		}
		above := templateFile(q)
		var added []string
		for _, role := range c.Roles {
			if !slices.Contains(p.Roles, role) && !slices.Contains(added, role) {
				added = append(added, role)
			}
		}
		why := "a list admits anyone with one of its roles"
		var fix string
		switch {
		case star:
			why = "every rule on the path applies"
			fix = fmt.Sprintf("remove this <ssr:access>: the rule in %s already decides who may open the page", above)
		case len(added) > 0 && c.Guard:
			fix = fmt.Sprintf(`list only the roles this page adds, such as role=%q guard="true"`, strings.Join(added, ","))
		case len(added) > 0:
			fix = fmt.Sprintf("list only the roles this page adds, such as role=%q", strings.Join(added, ","))
		case c.Guard:
			fix = fmt.Sprintf(`keep one role with the guard, such as role=%q guard="true"`, p.Roles[0])
		default:
			fix = fmt.Sprintf("remove this <ssr:access>: the rule in %s already admits these roles", above)
		}
		msg := fmt.Sprintf("role=%q adds nothing to role=%q in %s: %s",
			strings.Join(c.Roles, ","), strings.Join(p.Roles, ","), above, why)
		return errs.At(fmt.Sprintf("%s:%d", templateFile(r.Path), c.Line), "E-GEN-053", msg, fix)
	}
	return nil
}

// includes reports whether every role of sub is in set.
func includes(set, sub []string) bool {
	for _, role := range sub {
		if !slices.Contains(set, role) {
			return false
		}
	}
	return true
}

// guardMethods returns the file:line of every method named Guard in the route's folder.
// A file with syntax errors is checked as far as it parses; the Go build reports the errors.
func guardMethods(r *Route) ([]string, error) {
	files, fset, err := goFiles(r.Dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, f := range files {
		for _, decl := range f.file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv != nil && fn.Name.Name == "Guard" {
				out = append(out, fmt.Sprintf("%s:%d", path.Join(folder(r.Path), f.name), fset.Position(fn.Pos()).Line))
			}
		}
	}
	return out, nil
}

// paramConflicts reports every parameter folder after the first, by name, among the parameter
// folders of one folder that lead to the routes found, parsed or not. A URL segment could
// match either of them.
func paramConflicts(found map[string]bool) Diagnostics {
	params := map[string]map[string]bool{}
	for p := range found {
		segs := segments(p)
		for i, seg := range segs {
			if !isParam(seg) {
				continue
			}
			parent := "/" + strings.Join(segs[:i], "/")
			if params[parent] == nil {
				params[parent] = map[string]bool{}
			}
			params[parent][seg] = true
		}
	}
	var out Diagnostics
	for parent, set := range params {
		names := slices.Sorted(maps.Keys(set))
		for _, name := range names[1:] {
			p := path.Join(parent, name)
			pos := folder(p)
			if found[p] {
				pos = templatePos(p)
			}
			out = append(out, diag(pos, "E-GEN-041", "%s has two parameter folders, %s and %s", folder(parent), names[0], name))
		}
	}
	return out
}

// keyConflicts reports every route, after the first by path, whose route key another route has.
// The key tells the page's forms and live values apart from those of the other pages on its path.
func keyConflicts(found map[string]bool) Diagnostics {
	byKey := map[string][]string{}
	for p := range found {
		byKey[routeKey(p)] = append(byKey[routeKey(p)], p)
	}
	var out Diagnostics
	for key, ps := range byKey {
		slices.Sort(ps)
		for _, p := range ps[1:] {
			out = append(out, diag(templatePos(p), "E-GEN-047", "%s and %s have the same route key %s", ps[0], p, key))
		}
	}
	return out
}

// badFolders reports every folder on the path of a route found whose name cannot be the name
// of an importable Go package. Generated code uses it as a package name and in the name of an
// import.
func badFolders(found map[string]bool) Diagnostics {
	var out Diagnostics
	seen := map[string]bool{}
	for p := range found {
		segs := segments(p)
		for i, seg := range segs {
			q := "/" + strings.Join(segs[:i+1], "/")
			if seen[q] || goFolder(seg) {
				continue
			}
			seen[q] = true
			pos := folder(q)
			if found[q] {
				pos = templatePos(q)
			}
			msg := "the folder name %q cannot be a Go package name"
			if seg == "s_" || seg == "n_" {
				msg = "the parameter folder %q has no parameter name"
			}
			out = append(out, diag(pos, "E-GEN-043", msg, seg))
		}
	}
	return out
}

// goFolder reports whether a folder on the path of a page can name its Go package: ASCII
// letters, digits and _, starting with a letter, not main, testdata or a Go keyword, and not
// s_ or n_ without a parameter name.
func goFolder(seg string) bool {
	for i := 0; i < len(seg); i++ {
		c := seg[i]
		if ascii := c == '_' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9'; !ascii {
			return false
		}
	}
	return token.IsIdentifier(seg) && seg[0] != '_' && seg != "main" && seg != "testdata" && seg != "s_" && seg != "n_"
}

// isParam reports whether a folder named seg is a URL parameter: s_name takes any one URL
// segment, n_name a decimal integer.
func isParam(seg string) bool {
	return len(seg) > 2 && (strings.HasPrefix(seg, "s_") || strings.HasPrefix(seg, "n_"))
}

func segments(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// folder returns the folder of route path p relative to the app root, such as pages/notes.
func folder(p string) string { return path.Join("pages", p) }

// templateFile returns the template of route path p relative to the app root.
func templateFile(p string) string { return path.Join(folder(p), "index.html") }

func templatePos(p string) string { return templateFile(p) + ":1" }
