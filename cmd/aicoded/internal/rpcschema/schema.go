// Package rpcschema is the schema snapshot of the functions an app serves to other apps: the
// input and output type of each function, and the number each field of those types has on the
// wire. An app's own snapshot is .aicoded/rpc.json; an app that calls it holds a copy in
// .aicoded/services/<app>.json.
package rpcschema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"google.golang.org/protobuf/encoding/protowire"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/manifest"
)

// Schema is the snapshot of one app's functions.
type Schema struct {
	App     string            `json:"app"`
	Methods map[string]Method `json:"methods"`
	Types   map[string]Type   `json:"types"`
}

// Method is one function: the names of its input and output types.
type Method struct {
	In  string `json:"in"`
	Out string `json:"out"`
}

// Type is a struct type: its fields by name, and the numbers of its removed fields, which are
// never used again.
type Type struct {
	Fields  map[string]Field `json:"fields"`
	Retired []int            `json:"retired,omitempty"`
}

// Field is one field of a type. Type is bool, int32, int64, uint32, uint64, float32, float64,
// string, bytes, time or the name of a type of the schema; "[]" and one of those for a slice;
// or "*" and one of those except bytes for an optional value.
type Field struct {
	Number int    `json:"number"`
	Type   string `json:"type"`
}

var scalars = map[string]bool{
	"bool": true, "int32": true, "int64": true, "uint32": true, "uint64": true,
	"float32": true, "float64": true, "string": true, "bytes": true, "time": true,
}

// shape splits a field type into its prefix ("", "[]" or "*") and its element, and reports
// whether the type is well formed.
func shape(typ string) (prefix, elem string, ok bool) {
	switch {
	case strings.HasPrefix(typ, "[]"):
		prefix, elem = "[]", typ[2:]
	case strings.HasPrefix(typ, "*"):
		prefix, elem = "*", typ[1:]
	default:
		elem = typ
	}
	ok = (scalars[elem] || token.IsIdentifier(elem)) && (prefix != "*" || elem != "bytes")
	return prefix, elem, ok
}

// Load reads and parses the snapshot at path.
func Load(path string) (Schema, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return Schema{}, err
	}
	return Parse(path, data)
}

// Marshal returns s as aicoded writes it: keys sorted, indented by two spaces, with a trailing
// newline.
func (s Schema) Marshal() []byte {
	out := Schema{App: s.App, Methods: s.Methods, Types: make(map[string]Type, len(s.Types))}
	if out.Methods == nil {
		out.Methods = map[string]Method{}
	}
	for name, t := range s.Types {
		if t.Fields == nil {
			t.Fields = map[string]Field{}
		}
		t.Retired = slices.Sorted(slices.Values(t.Retired))
		out.Types[name] = t
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		panic(err) // maps of strings and ints always marshal
	}
	return append(data, '\n')
}

const fix = "never edit a snapshot by hand: restore .aicoded/rpc.json from the version history, " +
	"or run aicoded rpc add <app> again for a copy in .aicoded/services/"

// Parse reads the snapshot data, read from path. It refuses anything aicoded does not write with
// E-RPC-005 at path:line: invalid JSON, an unknown or repeated key, a missing or invalid name, a
// bad type, a reference to a type the snapshot does not have, and a field number that is not
// valid, is used twice in a type or is also retired.
func Parse(path string, data []byte) (Schema, error) {
	p := &parser{path: path, data: data, dec: json.NewDecoder(bytes.NewReader(data))}
	p.dec.UseNumber()
	s, err := p.schema()
	if err != nil {
		return Schema{}, err
	}
	if _, err := p.dec.Token(); !errors.Is(err, io.EOF) {
		return Schema{}, p.fail(p.dec.InputOffset(), "it has more data after its end")
	}
	for _, r := range p.refs {
		if _, ok := s.Types[r.name]; !ok {
			return Schema{}, p.fail(r.off, "%s names the type %s, which it does not have", r.what, r.name)
		}
	}
	return s, nil
}

type parser struct {
	path string
	data []byte
	dec  *json.Decoder
	refs []ref
}

// ref is a place that names a type.
type ref struct {
	what, name string
	off        int64
}

// number is a field number or retired number, with where it is.
type number struct {
	n    int
	off  int64
	what string
}

func (p *parser) fail(off int64, format string, args ...any) error {
	line := 1 + bytes.Count(p.data[:min(max(off, 0), int64(len(p.data)))], []byte("\n"))
	return errs.At(fmt.Sprintf("%s:%d", p.path, line), "E-RPC-005",
		"the schema snapshot is not valid: "+fmt.Sprintf(format, args...), fix)
}

func (p *parser) token() (json.Token, error) {
	t, err := p.dec.Token()
	if err == nil {
		return t, nil
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, p.fail(int64(len(p.data)), "it ends too early")
	}
	off := p.dec.InputOffset()
	if se := (*json.SyntaxError)(nil); errors.As(err, &se) {
		off = se.Offset
	}
	return nil, p.fail(off, "it is not valid JSON: %v", err)
}

// object reads an object and calls f for each key, with the offset just after the key.
func (p *parser) object(what string, f func(key string, off int64) error) error {
	t, err := p.token()
	if err != nil {
		return err
	}
	if t != json.Delim('{') {
		return p.fail(p.dec.InputOffset(), "%s is not an object", what)
	}
	seen := map[string]bool{}
	for p.dec.More() {
		t, err := p.token()
		if err != nil {
			return err
		}
		key, _ := t.(string)
		off := p.dec.InputOffset()
		if seen[key] {
			return p.fail(off, "%s has the key %q twice", what, key)
		}
		seen[key] = true
		if err := f(key, off); err != nil {
			return err
		}
	}
	_, err = p.token()
	return err
}

func (p *parser) str(what string) (string, int64, error) {
	t, err := p.token()
	if err != nil {
		return "", 0, err
	}
	off := p.dec.InputOffset()
	s, ok := t.(string)
	if !ok {
		return "", 0, p.fail(off, "%s is not a string", what)
	}
	return s, off, nil
}

func (p *parser) int(what string) (int, int64, error) {
	t, err := p.token()
	if err != nil {
		return 0, 0, err
	}
	off := p.dec.InputOffset()
	n, ok := t.(json.Number)
	if !ok {
		return 0, 0, p.fail(off, "%s is not a number", what)
	}
	i, err := strconv.Atoi(n.String())
	if err != nil {
		return 0, 0, p.fail(off, "%s is not a whole number", what)
	}
	return i, off, nil
}

func (p *parser) schema() (Schema, error) {
	s := Schema{Methods: map[string]Method{}, Types: map[string]Type{}}
	err := p.object("the snapshot", func(key string, off int64) error {
		switch key {
		case "app":
			app, off, err := p.str("app")
			if err == nil && !manifest.ValidApp(app) {
				err = p.fail(off, "the app name %q is not valid", app)
			}
			s.App = app
			return err
		case "methods":
			return p.object("methods", func(name string, off int64) error {
				if !token.IsIdentifier(name) || !token.IsExported(name) {
					return p.fail(off, "the function name %q is not an exported Go name", name)
				}
				m, err := p.method(name, off)
				s.Methods[name] = m
				return err
			})
		case "types":
			return p.object("types", func(name string, off int64) error {
				if !token.IsIdentifier(name) || scalars[name] {
					return p.fail(off, "the type name %q is not valid", name)
				}
				t, err := p.typ(name)
				s.Types[name] = t
				return err
			})
		}
		return p.fail(off, "the key %q is unknown", key)
	})
	if err == nil && s.App == "" {
		err = p.fail(0, "it names no app")
	}
	return s, err
}

func (p *parser) method(name string, at int64) (Method, error) {
	var m Method
	err := p.object("function "+name, func(key string, off int64) error {
		dst := map[string]*string{"in": &m.In, "out": &m.Out}[key]
		if dst == nil {
			return p.fail(off, "function %s has the unknown key %q", name, key)
		}
		v, off, err := p.str(key)
		if err != nil {
			return err
		}
		if !token.IsIdentifier(v) || scalars[v] {
			return p.fail(off, "the %s type of %s is %q, not the name of a type", key, name, v)
		}
		*dst = v
		p.refs = append(p.refs, ref{what: "function " + name, name: v, off: off})
		return nil
	})
	if err == nil && (m.In == "" || m.Out == "") {
		err = p.fail(at, "function %s needs both in and out", name)
	}
	return m, err
}

func (p *parser) typ(name string) (Type, error) {
	t := Type{Fields: map[string]Field{}}
	var fields, retired []number
	err := p.object("type "+name, func(key string, off int64) error {
		switch key {
		case "fields":
			return p.object("the fields of "+name, func(field string, off int64) error {
				if !token.IsIdentifier(field) || !token.IsExported(field) {
					return p.fail(off, "the field name %s.%q is not an exported Go name", name, field)
				}
				f, n, err := p.field(name+"."+field, off)
				t.Fields[field] = f
				fields = append(fields, n)
				return err
			})
		case "retired":
			return p.array("the retired numbers of "+name, func() error {
				n, off, err := p.int("a retired number of " + name)
				t.Retired = append(t.Retired, n)
				retired = append(retired, number{n: n, off: off, what: "the retired numbers of " + name})
				return err
			})
		}
		return p.fail(off, "type %s has the unknown key %q", name, key)
	})
	if err != nil {
		return Type{}, err
	}
	used := map[int]string{}
	for _, x := range slices.Concat(retired, fields) {
		if x.n < 1 || x.n > int(protowire.MaxValidNumber) ||
			(x.n >= int(protowire.FirstReservedNumber) && x.n <= int(protowire.LastReservedNumber)) {
			return Type{}, p.fail(x.off, "%s has the number %d, which is not a valid field number", x.what, x.n)
		}
		if other, ok := used[x.n]; ok {
			return Type{}, p.fail(x.off, "%s has the number %d, which %s has too", x.what, x.n, other)
		}
		used[x.n] = x.what
	}
	return t, nil
}

func (p *parser) field(name string, at int64) (Field, number, error) {
	var f Field
	n := number{off: at, what: "field " + name}
	err := p.object("field "+name, func(key string, off int64) error {
		var err error
		switch key {
		case "number":
			f.Number, n.off, err = p.int("the number of " + name)
			n.n = f.Number
			return err
		case "type":
			var off int64
			f.Type, off, err = p.str("the type of " + name)
			if err != nil {
				return err
			}
			_, elem, ok := shape(f.Type)
			if !ok {
				return p.fail(off, "field %s has the type %q, which is not allowed", name, f.Type)
			}
			if !scalars[elem] {
				p.refs = append(p.refs, ref{what: "field " + name, name: elem, off: off})
			}
			return nil
		}
		return p.fail(off, "field %s has the unknown key %q", name, key)
	})
	if err == nil && f.Type == "" {
		err = p.fail(at, "field %s has no type", name)
	}
	return f, n, err
}

// array reads an array and calls f for each element.
func (p *parser) array(what string, f func() error) error {
	t, err := p.token()
	if err != nil {
		return err
	}
	if t != json.Delim('[') {
		return p.fail(p.dec.InputOffset(), "%s is not a list", what)
	}
	for p.dec.More() {
		if err := f(); err != nil {
			return err
		}
	}
	_, err = p.token()
	return err
}
