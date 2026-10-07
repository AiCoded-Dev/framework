// Package yamldoc reads the one YAML document of a permission list, and source lines out of
// YAML errors.
package yamldoc

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strconv"

	"go.yaml.in/yaml/v3"

	"aicoded.dev/framework/internal/errs"
)

// Document returns the YAML document of the permission list data read from path, or an empty
// node when data holds none. It refuses with E-MAN-003 at path:line data that is not valid YAML,
// that holds more than one document, or that uses an anchor, an alias or a merge key, which it
// reports on every line that holds one. Its error unwraps, with Unwrap() []error, to the coded
// errors.
func Document(path string, data []byte) (yaml.Node, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc, next yaml.Node
	if err := dec.Decode(&doc); errors.Is(err, io.EOF) {
		return yaml.Node{}, nil
	} else if err != nil {
		return yaml.Node{}, errors.Join(errs.At(errPos(path, err), "E-MAN-003", "aicoded.yaml is not valid YAML: "+err.Error(), "fix the YAML syntax"))
	}
	if err := dec.Decode(&next); !errors.Is(err, io.EOF) {
		pos := fmt.Sprintf("%s:%d", path, next.Line)
		if err != nil {
			pos = errPos(path, err)
		}
		return yaml.Node{}, errors.Join(errs.At(pos, "E-MAN-003", "aicoded.yaml has more than one YAML document",
			"keep one YAML document: remove the --- line and everything below it"))
	}
	if err := anchors(path, &doc); err != nil {
		return yaml.Node{}, err
	}
	return doc, nil
}

// anchors refuses every line of doc that holds an anchor, an alias or a merge key, so that no
// reader expands one. It walks Content only and never follows an alias.
func anchors(path string, doc *yaml.Node) error {
	lines := map[int]bool{}
	var walk func(n *yaml.Node, key bool)
	walk = func(n *yaml.Node, key bool) {
		if n.Anchor != "" || n.Kind == yaml.AliasNode || key && n.Kind == yaml.ScalarNode && n.ShortTag() == "!!merge" {
			lines[n.Line] = true
		}
		for i, child := range n.Content {
			walk(child, n.Kind == yaml.MappingNode && i%2 == 0)
		}
	}
	walk(doc, false)
	var out []error
	for _, line := range slices.Sorted(maps.Keys(lines)) {
		out = append(out, errs.At(fmt.Sprintf("%s:%d", path, line), "E-MAN-003", "aicoded.yaml uses an anchor, alias or merge key",
			"write each value out in full: aicoded.yaml may not use &name, *name or <<"))
	}
	return errors.Join(out...)
}

func errPos(path string, err error) string {
	return fmt.Sprintf("%s:%d", path, LineOr1(err))
}

var lineRe = regexp.MustCompile(`line (\d+)`)

// Line returns the first line number a YAML error mentions.
func Line(err error) (int, bool) {
	m := lineRe.FindStringSubmatch(err.Error())
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

// LineOr1 returns the first line number a YAML error mentions, or 1 when it mentions none.
func LineOr1(err error) int {
	if line, ok := Line(err); ok {
		return line
	}
	return 1
}
