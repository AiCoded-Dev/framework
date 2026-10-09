package gotool

import (
	"bytes"
	"cmp"
	"encoding/json"
	"maps"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/lint"
)

const (
	codeBuild   = "E-CHK-002"
	codeVet     = "E-CHK-003"
	codeTest    = "E-CHK-004"
	codeStopped = "E-CHK-008"

	fixBuild   = "fix the Go code at this line"
	fixGo      = "fix what the go command reports in this message"
	fixVet     = "fix the code as the message says; go vet will run in the security checks too"
	fixTest    = "fix the code or the test until go test passes"
	fixStopped = "do not signal, kill or wait on other processes in tests; when no test does, or no test ran, give the go command more memory, then run the command again"

	// maxOutput is how much of the go command's output one problem carries.
	maxOutput = 4 << 10
	// maxTestLines is how many lines of a failed test's output its problem carries.
	maxTestLines = 20
)

var (
	// compileLine is a line the compiler or the go command prints about a place in a file.
	compileLine = regexp.MustCompile(`^((?:[A-Za-z]:)?[^:]+):(\d+)(?::\d+)?: (.*)$`)
	// posn is a position go vet prints.
	posn = regexp.MustCompile(`^(.+?):(\d+)(?::\d+)?$`)
	// testPos is the first place in a test file that a test's output names.
	testPos = regexp.MustCompile(`([^\s:]*_test\.go):(\d+)`)
	// missingPackage is what the go command says of an import of the framework's path that no
	// module provides, with the import path.
	missingPackage = regexp.MustCompile(`(?:does not contain package|no required module provides package|` +
		`cannot find module providing package|cannot find package) "?(` +
		regexp.QuoteMeta(lint.Framework) + `/[^\s";:]+)`)
)

// event is one JSON line of go build -json and go test -json.
type event struct {
	Action      string
	ImportPath  string
	Package     string
	Test        string
	Output      string
	FailedBuild string
}

// events decodes the JSON lines of out and returns the other lines as text.
func events(out []byte) ([]event, string) {
	var evs []event
	var text strings.Builder
	for line := range bytes.Lines(out) {
		var e event
		if bytes.HasPrefix(line, []byte("{")) && json.Unmarshal(line, &e) == nil {
			evs = append(evs, e)
			continue
		}
		text.Write(line)
	}
	return evs, text.String()
}

// builds gathers the build output of go build -json and go test -json by package.
type builds struct {
	order  []string
	output map[string]string
	failed map[string]bool
}

func newBuilds() *builds {
	return &builds{output: map[string]string{}, failed: map[string]bool{}}
}

// add takes e when it is a build event, and reports whether it was.
func (b *builds) add(e event) bool {
	if e.Action != "build-output" && e.Action != "build-fail" {
		return false
	}
	if _, ok := b.output[e.ImportPath]; !ok {
		b.order = append(b.order, e.ImportPath)
	}
	b.output[e.ImportPath] += e.Output
	if e.Action == "build-fail" {
		b.failed[e.ImportPath] = true
	}
	return true
}

// problems returns an E-CHK-002 for each place a package that failed to build names, and one
// without a position for such a package that names none.
func (b *builds) problems(r result) []*errs.Error {
	var ps []*errs.Error
	for _, pkg := range b.order {
		if !b.failed[pkg] {
			continue
		}
		found, rest := compileErrors(r.dir, b.output[pkg])
		if len(found) == 0 {
			found = append(found, unexplained(r.cmd+" of "+pkg, rest))
		}
		ps = append(ps, found...)
	}
	return ps
}

// compileErrors returns an E-CHK-002 for each line of text that names a place in a file, with
// the indented lines after it, and the other lines. It skips the # lines that name packages.
func compileErrors(dir, text string) ([]*errs.Error, string) {
	var ps []*errs.Error
	var rest strings.Builder
	for line := range strings.Lines(text) {
		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "" || strings.HasPrefix(line, "#"):
		case strings.HasPrefix(line, "\t") || strings.HasPrefix(line, " "):
			if len(ps) > 0 {
				ps[len(ps)-1].Msg += "\n" + line
			} else {
				rest.WriteString(line + "\n")
			}
		default:
			m := compileLine.FindStringSubmatch(strings.TrimPrefix(line, "vet: "))
			if m == nil {
				rest.WriteString(line + "\n")
				continue
			}
			ps = append(ps, errs.At(relPath(dir, m[1])+":"+m[2], codeBuild, m[3], fixBuild))
		}
	}
	blocks := lint.Blocks()
	for _, p := range ps {
		frameworkHint(p, blocks)
	}
	return ps, rest.String()
}

// frameworkHint adds to p, when its message says that no module provides an import of the
// framework's path that is not one of blocks, the packages of blocks whose last path element is
// that import's, and a fix that imports them instead.
func frameworkHint(p *errs.Error, blocks []string) {
	m := missingPackage.FindStringSubmatch(p.Msg)
	if m == nil || slices.Contains(blocks, m[1]) {
		return
	}
	name := path.Base(m[1])
	var named, quoted []string
	for _, b := range blocks {
		if path.Base(b) == name {
			named = append(named, b)
			quoted = append(quoted, strconv.Quote(b))
		}
	}
	switch len(named) {
	case 0:
		return
	case 1:
		p.Msg += "\nthe framework has no package " + m[1] + "; its " + name + " package is " + named[0]
	default:
		p.Msg += "\nthe framework has no package " + m[1] + "; its " + name + " packages are " + strings.Join(named, " and ")
	}
	p.Fix = "import " + strings.Join(quoted, " or ") + " instead"
}

// unexplained is the problem of a run of what that failed with no place in a file to show.
func unexplained(what, text string) *errs.Error {
	msg := what + " failed"
	if text = limit(text); text != "" {
		msg += ": " + text
	}
	return errs.New(codeBuild, msg, fixGo)
}

// finish adds the compile errors in text, what r printed outside its JSON, when r failed, and one
// problem for a failed run that nothing else explains. It sorts the problems by position. A run
// that a signal ended has the one problem E-CHK-008 instead, since what it printed is cut short.
func finish(r result, ps []*errs.Error, text string) []*errs.Error {
	if e := r.stopped(); e != nil {
		return []*errs.Error{e}
	}
	if r.failed {
		found, rest := compileErrors(r.dir, text)
		ps = append(ps, found...)
		if len(ps) == 0 {
			ps = append(ps, unexplained(r.cmd, rest))
		}
	}
	slices.SortStableFunc(ps, func(a, b *errs.Error) int {
		fa, la := splitPos(a.Pos)
		fb, lb := splitPos(b.Pos)
		return cmp.Or(cmp.Compare(fa, fb), cmp.Compare(la, lb))
	})
	return ps
}

// buildProblems turns what go build -json printed into problems.
func buildProblems(r result) []*errs.Error {
	evs, text := events(r.stdout)
	b := newBuilds()
	for _, e := range evs {
		b.add(e)
	}
	return finish(r, b.problems(r), text+string(r.stderr))
}

// vetProblems turns what go vet -json printed into problems: an E-CHK-003 for each finding and an
// E-CHK-002 for each compile error. The go command prints the findings on stdout from Go 1.26 on,
// and on stderr before; it prints compile errors on stderr. A run that passes but printed anything
// besides findings, package headers and download notices is an E-CHK-002 too, since findings it
// printed in another form would go unread.
func vetProblems(r result) []*errs.Error {
	ps, text := vetOutput(r.dir, r.stdout)
	found, rest := vetOutput(r.dir, r.stderr)
	ps, text = append(ps, found...), text+rest
	if unread := unreadVet(text); !r.failed && unread != "" {
		msg := "go vet printed what aicoded check cannot read, so its findings are unknown: " + limit(unread)
		ps = append(ps, errs.New(codeBuild, msg, fixGo))
	}
	return finish(r, ps, text)
}

// unreadVet returns the lines of text, what go vet -json printed outside its findings, that are
// not blank, package headers or download notices.
func unreadVet(text string) string {
	var unread strings.Builder
	for line := range strings.Lines(text) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(line, "# ") || strings.HasPrefix(trimmed, "go: downloading ") {
			continue
		}
		unread.WriteString(line)
	}
	return unread.String()
}

// vetOutput returns the findings in the JSON objects that go vet -json printed in out, and the
// other lines, an object left open included.
func vetOutput(dir string, out []byte) ([]*errs.Error, string) {
	var ps []*errs.Error
	var text strings.Builder
	var block []string
	for line := range strings.Lines(string(out)) {
		trimmed := strings.TrimRight(line, "\r\n")
		switch {
		case block != nil:
			block = append(block, line)
			if trimmed == "}" {
				found, ok := vetFindings(dir, strings.Join(block, ""))
				if !ok {
					text.WriteString(strings.Join(block, ""))
				}
				ps = append(ps, found...)
				block = nil
			}
		case trimmed == "{":
			block = []string{line}
		case strings.HasPrefix(trimmed, "{"):
			found, ok := vetFindings(dir, trimmed)
			if !ok {
				text.WriteString(line)
			}
			ps = append(ps, found...)
		default:
			text.WriteString(line)
		}
	}
	text.WriteString(strings.Join(block, ""))
	return ps, text.String()
}

// vetFindings returns the findings in one JSON object go vet -json printed, and false when data
// is not such an object.
func vetFindings(dir, data string) ([]*errs.Error, bool) {
	var tree map[string]map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &tree); err != nil {
		return nil, false
	}
	var ps []*errs.Error
	for _, pkg := range slices.Sorted(maps.Keys(tree)) {
		for _, analyzer := range slices.Sorted(maps.Keys(tree[pkg])) {
			raw := tree[pkg][analyzer]
			var diags []struct {
				Posn    string `json:"posn"`
				Message string `json:"message"`
			}
			if json.Unmarshal(raw, &diags) != nil {
				var failed struct {
					Error string `json:"error"`
				}
				_ = json.Unmarshal(raw, &failed)
				ps = append(ps, errs.New(codeVet, analyzer+": "+failed.Error, fixVet))
				continue
			}
			for _, d := range diags {
				msg := analyzer + ": " + d.Message
				if m := posn.FindStringSubmatch(d.Posn); m != nil {
					ps = append(ps, errs.At(relPath(dir, m[1])+":"+m[2], codeVet, msg, fixVet))
				} else {
					ps = append(ps, errs.New(codeVet, msg, fixVet))
				}
			}
		}
	}
	return ps, true
}

// testProblems turns what go test -json printed for the module module into problems: an
// E-CHK-004 for each top-level test that failed, one for a package that failed with no failed
// test, and an E-CHK-002 for each compile error.
func testProblems(r result, module string) []*errs.Error {
	evs, text := events(r.stdout)
	b := newBuilds()
	var order []string
	pkgs := map[string]*testPackage{}
	for _, e := range evs {
		if b.add(e) || e.Package == "" {
			continue
		}
		p := pkgs[e.Package]
		if p == nil {
			p = &testPackage{done: map[string]bool{}}
			pkgs[e.Package] = p
			order = append(order, e.Package)
		}
		p.add(e)
	}
	ps := b.problems(r)
	for _, name := range order {
		ps = append(ps, pkgs[name].problems(r.dir, packageDir(r.dir, module, name), name)...)
	}
	return finish(r, ps, text+string(r.stderr))
}

// testPackage is what the tests of one package printed.
type testPackage struct {
	output []testOutput
	failed []string        // the top-level tests that failed, in order
	done   map[string]bool // the top-level tests that ended
	fail   bool            // the package failed
	build  bool            // the package failed to build
}

// testOutput is output of the top-level test test, or of the package when test is "".
type testOutput struct{ test, text string }

func (p *testPackage) add(e event) {
	top, _, _ := strings.Cut(e.Test, "/")
	switch {
	case e.Action == "output":
		p.output = append(p.output, testOutput{top, e.Output})
	case e.Test == "":
		if e.Action == "fail" {
			p.fail, p.build = true, e.FailedBuild != ""
		}
	case e.Test != top:
	case e.Action == "fail":
		p.failed = append(p.failed, top)
		p.done[top] = true
	case e.Action == "pass" || e.Action == "skip":
		p.done[top] = true
	}
}

func (p *testPackage) problems(dir, pkgDir, name string) []*errs.Error {
	var ps []*errs.Error
	for _, test := range p.failed {
		ps = append(ps, testFailure(dir, pkgDir, test, p.text(func(t string) bool { return t == test })))
	}
	if p.fail && !p.build && len(p.failed) == 0 {
		ps = append(ps, testFailure(dir, pkgDir, "package "+name, p.text(func(t string) bool { return !p.done[t] })))
	}
	return ps
}

// text returns the output of the tests keep keeps, with the package's own output under "".
func (p *testPackage) text(keep func(test string) bool) string {
	var b strings.Builder
	for _, o := range p.output {
		if keep(o.test) {
			b.WriteString(o.text)
		}
	}
	return b.String()
}

// testFailure is the problem of what, a test or a package, that failed with output text. Its
// position is the first place in a test file the output names.
func testFailure(dir, pkgDir, what, text string) *errs.Error {
	lines := testLines(text)
	msg := what + " failed"
	if len(lines) > 0 {
		msg += ": " + limit(strings.Join(lines[:min(len(lines), maxTestLines)], "\n"))
	}
	m := testPos.FindStringSubmatch(strings.Join(lines, "\n"))
	if m == nil {
		return errs.New(codeTest, msg, fixTest)
	}
	file := m[1]
	if !filepath.IsAbs(file) {
		file = filepath.Join(pkgDir, file)
	}
	return errs.At(relPath(dir, file)+":"+m[2], codeTest, msg, fixTest)
}

// testLines returns the lines of a test's output without the lines go test frames it with.
func testLines(text string) []string {
	var lines []string
	for line := range strings.Lines(text) {
		line = strings.TrimRight(line, "\r\n")
		t := strings.TrimSpace(line)
		if t == "" || t == "PASS" || t == "FAIL" || slices.ContainsFunc(framing, func(p string) bool { return strings.HasPrefix(t, p) }) {
			continue
		}
		lines = append(lines, strings.TrimPrefix(line, "    "))
	}
	return lines
}

// framing are the starts of the lines go test prints around the output of tests.
var framing = []string{"=== RUN", "=== PAUSE", "=== CONT", "=== NAME", "--- PASS: ", "--- FAIL: ", "--- SKIP: ", "FAIL\t", "ok  \t", "?   \t"}

// packageDir returns the folder of the package pkg of the module module in dir.
func packageDir(dir, module, pkg string) string {
	if rest, ok := strings.CutPrefix(pkg, module+"/"); ok && module != "" {
		return filepath.Join(dir, filepath.FromSlash(rest))
	}
	return dir
}

// relPath returns file, which is absolute or relative to dir, relative to dir with slashes, or
// absolute when it is outside dir.
func relPath(dir, file string) string {
	if !filepath.IsAbs(file) {
		file = filepath.Join(dir, file)
	}
	rel, err := filepath.Rel(dir, file)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(file)
	}
	return filepath.ToSlash(rel)
}

// splitPos splits a file:line position; a problem without one has file "".
func splitPos(pos string) (string, int) {
	i := strings.LastIndexByte(pos, ':')
	if i < 0 {
		return pos, 0
	}
	n, _ := strconv.Atoi(pos[i+1:])
	return pos[:i], n
}

// limit trims s and cuts it to maxOutput bytes at a character boundary.
func limit(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxOutput {
		return s
	}
	n := maxOutput
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + " …"
}

// parseModule returns the module path of the go.mod data, or "".
func parseModule(data []byte) string {
	for line := range strings.Lines(string(data)) {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module")
		if !ok || rest == "" || (rest[0] != ' ' && rest[0] != '\t') {
			continue
		}
		rest, _, _ = strings.Cut(rest, "//")
		rest = strings.TrimSpace(rest)
		if p, err := strconv.Unquote(rest); err == nil {
			return p
		}
		return rest
	}
	return ""
}
