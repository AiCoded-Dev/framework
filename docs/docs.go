// Package docs holds the pages aicoded shows: the guides, the task recipes, the error catalogue
// and the changelog for agents.
package docs

import (
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"strings"

	"aicoded.dev/framework/internal/errs"
)

// FS holds guides/<slug>.md, tasks/<slug>.md, errors/<CODE>.md for every error code,
// errors/README.md and changelog-agents.md.
//
//go:embed guides/*.md tasks/*.md errors/*.md changelog-agents.md
var FS embed.FS

// Topic is a guide, a task recipe or the changelog for agents.
type Topic struct {
	// Path names the topic: its file under docs/ without .md, such as guides/forms, or
	// changelog.
	Path string
	// Title is the text of the file's first line, after "# ".
	Title string
	// Summary is the paragraph after the title, on one line.
	Summary string
}

const changelog = "changelog"

var (
	slug = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	// notProse matches the start of a heading, a code block, a list, a table, a quote or HTML.
	notProse = regexp.MustCompile("^(#|```|[-*+] |[0-9]+[.)] |[|><])")
)

// Topics returns the guides and the tasks, sorted by path, then the changelog.
func Topics() ([]Topic, error) {
	return topics(FS)
}

// Read returns the text of the topic at path. It refuses, with E-CLI-001, a path that Topics
// does not list.
func Read(path string) (string, error) {
	ts, err := Topics()
	if err != nil {
		return "", err
	}
	for _, t := range ts {
		if t.Path == path {
			data, err := fs.ReadFile(FS, file(path))
			return string(data), err
		}
	}
	return "", errs.New("E-CLI-001", fmt.Sprintf("there is no topic %q", path),
		"copy the topic from the list that aicoded explain or howto gives with no topic, such as guides/overview")
}

func file(path string) string {
	if path == changelog {
		return "changelog-agents.md"
	}
	return path + ".md"
}

func topics(fsys fs.FS) ([]Topic, error) {
	var out []Topic
	for _, dir := range []string{"guides", "tasks"} {
		files, err := fs.Glob(fsys, dir+"/*.md")
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			name := strings.TrimSuffix(f[len(dir)+1:], ".md")
			if !slug.MatchString(name) {
				return nil, fmt.Errorf("docs/%s: name the file in lowercase words joined by -", f)
			}
			t, err := readTopic(fsys, dir+"/"+name)
			if err != nil {
				return nil, err
			}
			out = append(out, t)
		}
	}
	t, err := readTopic(fsys, changelog)
	if err != nil {
		return nil, err
	}
	return append(out, t), nil
}

func readTopic(fsys fs.FS, path string) (Topic, error) {
	data, err := fs.ReadFile(fsys, file(path))
	if err != nil {
		return Topic{}, err
	}
	title, summary, err := parse("docs/"+file(path), string(data))
	return Topic{Path: path, Title: title, Summary: summary}, err
}

// parse returns the title and the summary of the Markdown text of the file name: its first line
// is "# <Title>", its second is blank, and the summary is the paragraph that follows, which
// holds no link and ends with a blank line before any heading, code block, list, table, quote or
// HTML.
func parse(name, text string) (title, summary string, err error) {
	lines := strings.Split(text, "\n")
	title, ok := strings.CutPrefix(lines[0], "# ")
	if !ok || strings.TrimSpace(title) == "" {
		return "", "", fmt.Errorf("%s:1: start the file with # <Title>", name)
	}
	title = strings.TrimSpace(title)
	if len(lines) < 3 || lines[1] != "" || strings.TrimSpace(lines[2]) == "" || notProse.MatchString(strings.TrimSpace(lines[2])) {
		return "", "", fmt.Errorf("%s:3: follow the title with a blank line and a summary paragraph", name)
	}
	var para []string
	for i, l := range lines[2:] {
		l = strings.TrimSpace(l)
		if l == "" {
			break
		}
		if i > 0 && notProse.MatchString(l) {
			return "", "", fmt.Errorf("%s:%d: end the summary paragraph with a blank line before this line", name, i+3)
		}
		para = append(para, l)
	}
	summary = strings.Join(para, " ")
	if strings.Contains(summary, "](") {
		return "", "", fmt.Errorf("%s:3: move the link out of the summary, which howto and llms.txt show alone", name)
	}
	return title, summary, nil
}
