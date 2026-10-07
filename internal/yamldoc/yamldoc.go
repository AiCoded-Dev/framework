// Package yamldoc reads the one YAML document of a permission list, and source lines out of
// YAML errors.
package yamldoc

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"

	"go.yaml.in/yaml/v3"

	"aicoded.dev/framework/internal/errs"
)

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
	line, ok := Line(err)
	if !ok {
		line = 1
	}
	return fmt.Sprintf("%s:%d", path, line)
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
