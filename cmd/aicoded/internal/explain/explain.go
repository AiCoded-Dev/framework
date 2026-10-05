// Package explain reads the docs built into aicoded: the error catalogue, the guides, the task
// recipes and the changelog for agents.
package explain

import (
	"fmt"
	"io/fs"
	"regexp"
	"strings"

	"aicoded.dev/framework/docs"
	"aicoded.dev/framework/internal/errs"
)

// Entry is one page of the error catalogue.
type Entry struct {
	Code, Title string
}

var codeName = regexp.MustCompile(`^E-[A-Z]+-[0-9]{3}$`)

// List returns every page of the catalogue, sorted by code. Title is the text after "# CODE: "
// on the page's first line.
func List() ([]Entry, error) {
	files, err := fs.ReadDir(docs.FS, "errors")
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, f := range files {
		if f.Name() == "README.md" {
			continue
		}
		code := strings.TrimSuffix(f.Name(), ".md")
		data, err := fs.ReadFile(docs.FS, "errors/"+f.Name())
		if err != nil {
			return nil, err
		}
		first, _, _ := strings.Cut(string(data), "\n")
		title, ok := strings.CutPrefix(first, "# "+code+": ")
		if !codeName.MatchString(code) || !ok {
			return nil, fmt.Errorf("docs/errors/%s does not start with # %s: <title>", f.Name(), code)
		}
		out = append(out, Entry{Code: code, Title: title})
	}
	return out, nil
}

// Page returns the page of the error code, written in any case. It refuses, with E-CLI-001, a
// code that is not written like one or has no page.
func Page(code string) (string, error) {
	name := strings.ToUpper(code)
	if codeName.MatchString(name) {
		if data, err := fs.ReadFile(docs.FS, "errors/"+name+".md"); err == nil {
			return string(data), nil
		}
	}
	return "", errs.New("E-CLI-001", fmt.Sprintf("there is no error page for %q", code),
		"copy the code from the error, such as E-DEV-001; aicoded explain with no code lists them all")
}

// Read returns the page of the error code, for a topic that starts with E- in any case, and
// otherwise the guide, task or changelog that docs.Read returns. It refuses, with E-CLI-001, a
// topic that has no page.
func Read(topic string) (string, error) {
	if strings.HasPrefix(strings.ToUpper(topic), "E-") {
		return Page(topic)
	}
	return docs.Read(topic)
}
