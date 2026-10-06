package docs

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"aicoded.dev/framework/lint"
)

const stale = "run make docs and commit the result"

// The lines around the table of lint's rules, which make docs writes between them.
const (
	rulesStart = "<!-- rules -->"
	rulesEnd   = "<!-- end rules -->"
)

const (
	llmsSummary = "The open Go framework for company web apps that AI assistants build: type-checked " +
		"server-rendered pages, building blocks for data, files and mail, and calls between apps, " +
		"each app held to what its permission list allows."
	llmsIntro = "Start with the overview. `aicoded explain <topic>` and the `howto` tool of " +
		"`aicoded mcp` serve the same pages offline."
)

var (
	marker = regexp.MustCompile(`^(\s*)<!-- code: (\S+)(?: (\S+))? -->$`)
	fence  = regexp.MustCompile("^(\\s*)(```+)")
	link   = regexp.MustCompile(`\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	scheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)
	// tag matches the start of raw HTML: a tag, a comment, a declaration or a processing
	// instruction.
	tag = regexp.MustCompile(`<[a-zA-Z/!?][^<>\s]*>?`)
	// block matches a line that starts a heading, a list item or a table row, which ends the
	// paragraph before it.
	block = regexp.MustCompile(`^\s*(#|[-*+] |[0-9]+[.)] |\|)`)
)

// checkDocs runs every check of the docs on the repo at root and returns the problems found.
// With update it first rewrites llms.txt and the code blocks that differ from their source.
func checkDocs(root string, update bool) ([]string, error) {
	problems, err := checkLLMs(root, update)
	if err != nil {
		return nil, err
	}
	files, err := docFiles(root)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		code, err := checkCode(root, f, update)
		if err != nil {
			return nil, err
		}
		links, err := checkLinks(root, f)
		if err != nil {
			return nil, err
		}
		html, err := checkHTML(root, f)
		if err != nil {
			return nil, err
		}
		problems = append(append(append(problems, code...), links...), html...)
	}
	return problems, nil
}

// docFiles returns, relative to root, every Markdown file under it and llms.txt. It skips
// folders whose name starts with a dot, and testdata.
func docFiles(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if rel != "." && (strings.HasPrefix(d.Name(), ".") || d.Name() == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(rel, ".md") || rel == "llms.txt" {
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	return out, err
}

// checkCode checks every code block of the doc at rel that a marker ties to its source, and
// every table of lint's rules. With update it rewrites the blocks and tables that differ.
func checkCode(root, rel string, update bool) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	var problems []string
	changed := false
	open := ""
	for i := 0; i < len(lines); i++ {
		at := fmt.Sprintf("%s:%d: ", rel, i+1)
		if open != "" {
			if closes(lines[i], open) {
				open = ""
			}
			continue
		}
		if f := fence.FindStringSubmatch(lines[i]); f != nil {
			open = f[2]
			continue
		}
		if lines[i] == rulesStart {
			end := slices.Index(lines[i+1:], rulesEnd)
			if end < 0 {
				problems = append(problems, at+"close the table with "+rulesEnd)
				break
			}
			end += i + 1
			table, p := rulesTable(lint.Rules())
			switch {
			case p != "":
				problems = append(problems, at+p)
			case slices.Equal(lines[i+1:end], table):
			case update:
				lines = slices.Replace(lines, i+1, end, table...)
				end = i + 1 + len(table)
				changed = true
			default:
				problems = append(problems, at+"the table differs from lint's rules; "+stale)
			}
			i = end
			continue
		}
		m := marker.FindStringSubmatch(lines[i])
		if m == nil {
			text := withoutCode(lines[i])
			switch {
			case strings.Contains(text, "<!-- code:"):
				problems = append(problems, at+"write the marker as <!-- code: <path> [<name>] --> on a line of its own")
			case lines[i] == rulesEnd:
				problems = append(problems, at+rulesEnd+" closes no table; open the table with "+rulesStart+" on a line of its own")
			case strings.Contains(text, "<!-- rules") || strings.Contains(text, "<!-- end rules"):
				problems = append(problems, at+"write "+rulesStart+" and "+rulesEnd+" each on a line of its own, around the table")
			}
			continue
		}
		indent, path, name := m[1], m[2], m[3]
		var f []string
		if i+1 < len(lines) {
			f = fence.FindStringSubmatch(lines[i+1])
		}
		if f == nil || f[1] != indent {
			problems = append(problems, at+"follow the marker directly with a fenced code block")
			continue
		}
		end := i + 2
		for end < len(lines) && !closes(lines[end], f[2]) {
			end++
		}
		if end == len(lines) {
			problems = append(problems, fmt.Sprintf("%s:%d: close the code block", rel, i+2))
			break
		}
		block, p := quote(root, path, name, indent, f[2])
		switch {
		case p != "":
			problems = append(problems, at+p)
		case slices.Equal(lines[i+2:end], block):
		case update:
			lines = slices.Replace(lines, i+2, end, block...)
			end = i + 2 + len(block)
			changed = true
		default:
			problems = append(problems, at+"the code block differs from "+strings.TrimSpace(path+" "+name)+"; "+stale)
		}
		i = end
	}
	if !changed {
		return problems, nil
	}
	return problems, os.WriteFile(filepath.Join(root, rel), []byte(strings.Join(lines, "\n")), 0o644)
}

// rulesTable returns the lines of the table of rules for a guide in docs/guides, a row for each
// rule with its code linked to its page, or a problem when a text of a rule holds a character
// that Markdown reads or a line break, which would end its row.
func rulesTable(rules []lint.Rule) ([]string, string) {
	lines := []string{"| Code | Rule | Don't | Do |", "|---|---|---|---|"}
	for _, r := range rules {
		if strings.ContainsAny(r.Title+r.Dont+r.Do, "<>|*`[]\\\r\n") {
			return nil, r.Code + " in lint/rules.go holds a character that Markdown reads or a line break; write its title, Dont and Do on one line, without < > | * ` [ ] or \\"
		}
		lines = append(lines, "| ["+r.Code+"](../errors/"+r.Code+".md) | "+r.Title+" | "+r.Dont+" | "+r.Do+" |")
	}
	return lines, ""
}

// closes reports whether line closes a code block opened with the backticks of open.
func closes(line, open string) bool {
	l := strings.TrimSpace(line)
	return len(l) >= len(open) && strings.Trim(l, "`") == ""
}

// quote returns the lines of the code block that a marker names, indented by indent, or a
// problem. open is the fence that opens the block.
func quote(root, path, name, indent, open string) ([]string, string) {
	text, err := source(root, path, name)
	if err != nil {
		return nil, err.Error()
	}
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	for k, l := range lines {
		if closes(l, open) {
			return nil, path + " has a line of backticks that would close the block; open the block with a longer fence"
		}
		if l != "" {
			lines[k] = indent + l
		}
	}
	return lines, ""
}

// source returns, ending in a newline, the text a marker names: the whole file at path,
// relative to root, or the declaration name of that Go file with its doc comment. name is Func,
// Type, Var or Recv.Method.
func source(root, path, name string) (string, error) {
	if !filepath.IsLocal(filepath.FromSlash(path)) {
		return "", fmt.Errorf("%s is not a path in the repo; write it relative to the repo root", path)
	}
	src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%s does not exist; fix the path in the marker", path)
	}
	if err != nil {
		return "", err
	}
	if name == "" {
		text := string(src)
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		return text, nil
	}
	if !strings.HasSuffix(path, ".go") {
		return "", fmt.Errorf("%s is not a Go file, so the marker cannot name a declaration in it", path)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return "", fmt.Errorf("%s does not parse: %w", path, err)
	}
	start, end, ok := declaration(f, name)
	if !ok {
		return "", fmt.Errorf("%s has no declaration %s; fix the name in the marker", path, name)
	}
	from := fset.Position(start).Offset
	from = strings.LastIndexByte(string(src[:from]), '\n') + 1
	to := fset.Position(end).Offset
	if n := strings.IndexByte(string(src[to:]), '\n'); n >= 0 {
		to += n
	} else {
		to = len(src)
	}
	return string(src[from:to]) + "\n", nil
}

// declaration returns where the declaration name of f starts, with its doc comment, and where
// it ends. A type, var or const declared in a group is quoted alone.
func declaration(f *ast.File, name string) (start, end token.Pos, ok bool) {
	recv, method, isMethod := strings.Cut(name, ".")
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if isMethod && d.Recv != nil && d.Name.Name == method && receiver(d.Recv) == recv ||
				!isMethod && d.Recv == nil && d.Name.Name == name {
				return docStart(d.Doc, d.Pos()), d.End(), true
			}
		case *ast.GenDecl:
			for _, s := range d.Specs {
				if isMethod || !declares(s, name) {
					continue
				}
				if d.Lparen.IsValid() {
					return docStart(specDoc(s), s.Pos()), s.End(), true
				}
				return docStart(d.Doc, d.Pos()), d.End(), true
			}
		}
	}
	return token.NoPos, token.NoPos, false
}

func receiver(fl *ast.FieldList) string {
	t := fl.List[0].Type
	if s, ok := t.(*ast.StarExpr); ok {
		t = s.X
	}
	switch x := t.(type) {
	case *ast.IndexExpr:
		t = x.X
	case *ast.IndexListExpr:
		t = x.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

func declares(s ast.Spec, name string) bool {
	switch s := s.(type) {
	case *ast.TypeSpec:
		return s.Name.Name == name
	case *ast.ValueSpec:
		for _, n := range s.Names {
			if n.Name == name {
				return true
			}
		}
	}
	return false
}

func specDoc(s ast.Spec) *ast.CommentGroup {
	switch s := s.(type) {
	case *ast.TypeSpec:
		return s.Doc
	case *ast.ValueSpec:
		return s.Doc
	}
	return nil
}

func docStart(doc *ast.CommentGroup, pos token.Pos) token.Pos {
	if doc != nil {
		return doc.Pos()
	}
	return pos
}

// checkLinks checks that every relative link of the doc at rel names a file or folder in the
// repo at root. Links with a scheme, such as https:, and links to an anchor of the same doc are
// not checked; a link to an anchor of another file is refused.
func checkLinks(root, rel string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return nil, err
	}
	var problems []string
	open := ""
	for i, line := range strings.Split(string(data), "\n") {
		if open != "" {
			if closes(line, open) {
				open = ""
			}
			continue
		}
		if m := fence.FindStringSubmatch(line); m != nil {
			open = m[2]
			continue
		}
		for _, m := range link.FindAllStringSubmatch(withoutCode(line), -1) {
			if p := checkLink(root, rel, m[1]); p != "" {
				problems = append(problems, fmt.Sprintf("%s:%d: the link %s %s", rel, i+1, m[1], p))
			}
		}
	}
	return problems, nil
}

func checkLink(root, rel, target string) string {
	switch {
	case scheme.MatchString(target), strings.HasPrefix(target, "#"):
		return ""
	case strings.Contains(target, "#"):
		return "points into another file, which is not checked; link to the file"
	case strings.HasPrefix(target, "/"):
		return "starts at /; make it relative to this file"
	}
	path := filepath.Join(filepath.Dir(filepath.FromSlash(rel)), filepath.FromSlash(target))
	if !filepath.IsLocal(path) {
		return "points outside the repo"
	}
	if _, err := os.Stat(filepath.Join(root, path)); err != nil {
		return "names no file; fix the link or create the file"
	}
	return ""
}

// checkHTML checks that the doc at rel holds no raw HTML, which GitHub and the docs site drop: a
// < followed by a letter, /, ! or ?, outside code blocks and code spans. The markers that
// checkCode reads are allowed on lines of their own.
func checkHTML(root, rel string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	var problems []string
	open, first := "", 0
	for i, line := range lines {
		if open != "" {
			if closes(line, open) {
				open = ""
			}
			first = i + 1
			continue
		}
		f := fence.FindStringSubmatch(line)
		skip := f != nil || strings.TrimSpace(line) == "" || marker.MatchString(line) || line == rulesStart || line == rulesEnd
		if skip || block.MatchString(line) {
			problems = append(problems, rawHTML(rel, first, lines[first:i])...)
			first = i
			if skip {
				first = i + 1
			}
		}
		if f != nil {
			open = f[2]
		}
	}
	return append(problems, rawHTML(rel, first, lines[first:])...), nil
}

// rawHTML returns a problem for every piece of raw HTML in para, the lines of a paragraph of the
// doc rel from the line index first. A code span may run over the lines.
func rawHTML(rel string, first int, para []string) []string {
	var problems []string
	for k, line := range strings.Split(withoutCode(strings.Join(para, "\n")), "\n") {
		for _, m := range tag.FindAllString(line, -1) {
			problems = append(problems, fmt.Sprintf("%s:%d: Markdown reads %s as HTML and drops it; put the text in a code span", rel, first+k+1, m))
		}
	}
	return problems
}

// withoutCode returns text without its code spans. It keeps the line breaks of a span, so every
// line stays where it was.
func withoutCode(text string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(text, '`')
		if i < 0 {
			b.WriteString(text)
			return b.String()
		}
		b.WriteString(text[:i])
		n := i
		for n < len(text) && text[n] == '`' {
			n++
		}
		run, rest := text[i:n], text[n:]
		end := closingRun(rest, len(run))
		if end < 0 {
			b.WriteString(run)
			text = rest
			continue
		}
		b.WriteString(strings.Repeat("\n", strings.Count(rest[:end], "\n")))
		text = rest[end+len(run):]
	}
}

// closingRun returns the index in s of the first run of exactly n backticks, or -1.
func closingRun(s string, n int) int {
	for i := 0; i < len(s); {
		if s[i] != '`' {
			i++
			continue
		}
		j := i
		for j < len(s) && s[j] == '`' {
			j++
		}
		if j-i == n {
			return i
		}
		i = j
	}
	return -1
}

// checkLLMs checks that llms.txt at root is what llms makes. With update it writes it.
func checkLLMs(root string, update bool) ([]string, error) {
	want, err := llms(root)
	if err != nil {
		return []string{err.Error()}, nil
	}
	path := filepath.Join(root, "llms.txt")
	got, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	switch {
	case string(got) == want:
		return nil, nil
	case update:
		return nil, os.WriteFile(path, []byte(want), 0o644)
	}
	return []string{"llms.txt is out of date; " + stale}, nil
}

// llms returns the text of llms.txt for the repo at root: the guides, the tasks, the error
// catalogue, and the changelog and the examples' READMEs as optional reading.
func llms(root string) (string, error) {
	ts, err := topics(os.DirFS(filepath.Join(root, "docs")))
	if err != nil {
		return "", err
	}
	var guides, tasks, optional []string
	for _, t := range ts {
		e := item(t.Title, "docs/"+file(t.Path), t.Summary)
		switch {
		case strings.HasPrefix(t.Path, "guides/"):
			guides = append(guides, e)
		case strings.HasPrefix(t.Path, "tasks/"):
			tasks = append(tasks, e)
		default:
			optional = append(optional, e)
		}
	}
	errorsEntry, err := entry(root, "docs/errors/README.md")
	if err != nil {
		return "", err
	}
	readmes := []string{"examples/README.md"}
	more, err := filepath.Glob(filepath.Join(root, "examples", "*", "README.md"))
	if err != nil {
		return "", err
	}
	for _, m := range more {
		rel, err := filepath.Rel(root, m)
		if err != nil {
			return "", err
		}
		readmes = append(readmes, filepath.ToSlash(rel))
	}
	for _, r := range readmes {
		if _, err := os.Stat(filepath.Join(root, r)); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		e, err := entry(root, r)
		if err != nil {
			return "", err
		}
		optional = append(optional, e)
	}
	var b strings.Builder
	b.WriteString("# aicoded framework\n\n> " + llmsSummary + "\n\n" + llmsIntro + "\n")
	section(&b, "Guides", guides)
	section(&b, "Tasks", tasks)
	section(&b, "Errors", []string{errorsEntry})
	section(&b, "Optional", optional)
	return b.String(), nil
}

// entry returns the llms.txt entry of the Markdown file at rel, which starts like a topic.
func entry(root, rel string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return "", err
	}
	title, summary, err := parse(rel, string(data))
	return item(title, rel, summary), err
}

func item(title, path, summary string) string {
	return "- [" + title + "](" + path + "): " + summary
}

func section(b *strings.Builder, title string, entries []string) {
	if len(entries) > 0 {
		b.WriteString("\n## " + title + "\n\n" + strings.Join(entries, "\n") + "\n")
	}
}
