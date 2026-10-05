package generate

import (
	"bytes"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"aicoded.dev/framework/cmd/aicoded/internal/manifest"
	"aicoded.dev/framework/cmd/aicoded/internal/yamlerr"
	"aicoded.dev/framework/internal/errs"
)

// manifestFile is the app's permission list.
const manifestFile = "aicoded.yaml"

// accessMarker is the comment the generator writes above the access section.
const accessMarker = "# access is written by aicoded generate from <ssr:access>; do not edit it"

// accessEntry is what the permission list says about one page a viewer can open.
type accessEntry struct {
	Require []string
	Guard   bool
	Calls   []string
}

// accessMap returns the access entry of every page a viewer can open, keyed by URL pattern.
// require holds every rule on the path that does not admit every viewer, root first, each as
// its roles joined by "|"; guard tells whether a Guard runs on the path; calls names the calls
// of the routes that render the page. A layout with pages below it only redirects and has no
// entry.
func accessMap(routes []*Route) map[string]accessEntry {
	byPath := make(map[string]*Route, len(routes))
	for _, r := range routes {
		byPath[r.Path] = r
	}
	out := map[string]accessEntry{}
	for _, r := range routes {
		if r.Template.GetContentNode() != nil && hasPagesBelow(r.Path, byPath) {
			continue
		}
		var e accessEntry
		segs := segments(r.Path)
		for i := 0; i <= len(segs); i++ {
			x := byPath["/"+strings.Join(segs[:i], "/")]
			if x == nil || x.Template.Access() == nil {
				continue
			}
			a := x.Template.Access()
			e.Guard = e.Guard || a.Guard
			if slices.Contains(a.Roles, "*") {
				continue
			}
			rule := strings.Join(slices.Compact(slices.Sorted(slices.Values(a.Roles))), "|")
			if !slices.Contains(e.Require, rule) {
				e.Require = append(e.Require, rule)
			}
		}
		if len(e.Require) == 0 {
			e.Require = []string{"*"}
		}
		calls := map[string]bool{}
		for _, x := range chain(r.Path, byPath) {
			for _, c := range x.Template.Calls() {
				calls[c.Name] = true
			}
		}
		if len(calls) > 0 {
			e.Calls = slices.Sorted(maps.Keys(calls))
		}
		out[pattern(r.Path)] = e
	}
	return out
}

// pattern returns the URL pattern of the route at p: /notes/n_id becomes /notes/{id}.
func pattern(p string) string {
	segs := segments(p)
	for i, s := range segs {
		if isParam(s) {
			segs[i] = "{" + s[2:] + "}"
		}
	}
	return "/" + strings.Join(segs, "/")
}

// accessSection renders the access section of the permission list, marker comment included.
func accessSection(m map[string]accessEntry) ([]byte, error) {
	body := &yaml.Node{Kind: yaml.MappingNode}
	for _, k := range slices.Sorted(maps.Keys(m)) {
		e := m[k]
		v := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{plain("require"), list(e.Require)}}
		if e.Guard {
			v.Content = append(v.Content, plain("guard"), yes())
		}
		if len(e.Calls) > 0 {
			v.Content = append(v.Content, plain("calls"), list(e.Calls))
		}
		body.Content = append(body.Content, quoted(k), v)
	}
	return encodeSection("access", accessMarker, body)
}

// encodeSection renders the top-level key with the value body and the marker comment above it.
// An empty mapping is written as {}.
func encodeSection(key, marker string, body *yaml.Node) ([]byte, error) {
	if len(body.Content) == 0 {
		body.Style = yaml.FlowStyle
	}
	k := plain(key)
	k.HeadComment = marker
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(&yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{k, body}}); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func plain(s string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Value: s} }

func quoted(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Value: s, Style: yaml.DoubleQuotedStyle}
}

func yes() *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"} }

func list(items []string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
	for _, s := range items {
		n.Content = append(n.Content, quoted(s))
	}
	return n
}

// withAccess returns the permission list data with its access section replaced by section.
func withAccess(data, section []byte) ([]byte, error) {
	return withSection(data, "access", accessMarker, section)
}

// withSection returns the permission list data with its section under key replaced by section,
// or with section appended when it has none. The section runs from marker, the comment right
// above the key, or from the key itself up to the next line that starts at the left margin and is
// not a comment; comments and blank lines right before that line belong to what follows. Every
// other byte stays as it was. It refuses data it cannot change that way: the result must be one
// YAML document that says what data says, except that its section under key says what section
// says.
func withSection(data []byte, key, marker string, section []byte) ([]byte, error) {
	before, top, err := parseManifest(data)
	if err != nil {
		return nil, err
	}
	at := -1
	for i := 0; top != nil && i+1 < len(top.Content); i += 2 {
		if top.Content[i].Value == key {
			at = top.Content[i].Line - 1
		}
	}
	var out []byte
	if at < 0 {
		out = bytes.Clone(data)
		if len(out) > 0 && out[len(out)-1] != '\n' {
			out = append(out, '\n')
		}
		out = append(out, section...)
	} else {
		lines := bytes.SplitAfter(data, []byte("\n"))
		if len(lines[len(lines)-1]) == 0 {
			lines = lines[:len(lines)-1]
		}
		if at >= len(lines) {
			return nil, manifestError(at+1, `aicoded.yaml breaks lines with a lone carriage return or a Unicode line separator; end every line with \n`)
		}
		start, end := at, len(lines)
		if start > 0 && string(bytes.TrimRight(lines[start-1], "\r\n")) == marker {
			start--
		}
		for i := at + 1; i < len(lines); i++ {
			if l := lines[i]; !blankOrComment(l) && l[0] != ' ' && l[0] != '\t' {
				end = i
				break
			}
		}
		for end > at+1 && blankOrComment(lines[end-1]) {
			end--
		}
		out = slices.Concat(slices.Concat(lines[:start]...), section, slices.Concat(lines[end:]...))
	}
	want, _, err := parseManifest(section)
	if err != nil {
		return nil, err
	}
	after, _, err := parseManifest(out)
	if err != nil || !reflect.DeepEqual(after[key], want[key]) || !sameExcept(before, after, key) {
		return nil, manifestError(at+1, "aicoded generate cannot replace the "+key+" section of aicoded.yaml without changing the rest of it")
	}
	return out, nil
}

// sameExcept reports whether a and b hold the same values under every key but key.
func sameExcept(a, b map[string]any, key string) bool {
	a, b = maps.Clone(a), maps.Clone(b)
	delete(a, key)
	delete(b, key)
	return len(a) == 0 && len(b) == 0 || reflect.DeepEqual(a, b)
}

// blankOrComment reports whether line holds only white space or a comment that starts it.
func blankOrComment(line []byte) bool {
	t := bytes.TrimRight(line, " \t\r\n")
	return len(t) == 0 || t[0] == '#'
}

// parseManifest parses the permission list data, which must be one YAML document holding a
// mapping, or nothing. It returns the mapping's values and its node, which is nil when there is
// none.
func parseManifest(data []byte) (map[string]any, *yaml.Node, error) {
	doc, err := manifest.Document(manifestFile, data)
	if err != nil || doc.Kind == 0 {
		return nil, nil, err
	}
	var m map[string]any
	if err := doc.Decode(&m); err != nil {
		return nil, nil, manifestError(errLine(err), "aicoded.yaml has an unexpected shape: "+err.Error())
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return m, nil, nil
	}
	return m, doc.Content[0], nil
}

func errLine(err error) int {
	if line, ok := yamlerr.Line(err); ok {
		return line
	}
	return 1
}

// manifestError reports that aicoded.yaml cannot take the generated sections.
func manifestError(line int, msg string) error {
	return errs.At(fmt.Sprintf("%s:%d", manifestFile, max(line, 1)), "E-MAN-003", msg,
		"keep aicoded.yaml one YAML document in block style, with every top-level key at the start of a line and one access and one services section")
}
