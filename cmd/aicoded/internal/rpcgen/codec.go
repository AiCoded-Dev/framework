package rpcgen

import (
	"bytes"
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"aicoded.dev/framework/cmd/aicoded/internal/rpcschema"
)

// wireScalar is how the codec carries a scalar type of the snapshot: its Go type, the name of
// its wire functions (wire.Append<fn> and wire.Value.<fn>), the test that a value v is not the
// zero value, with %s standing for v, and, for a type a repeated field may send packed, the
// wire type of one element.
type wireScalar struct {
	goType, fn, nonZero, packed string
}

var wireScalars = map[string]wireScalar{
	"bool":    {"bool", "Bool", "%s", "protowire.VarintType"},
	"int32":   {"int32", "Int32", "%s != 0", "protowire.VarintType"},
	"int64":   {"int64", "Int64", "%s != 0", "protowire.VarintType"},
	"uint32":  {"uint32", "Uint32", "%s != 0", "protowire.VarintType"},
	"uint64":  {"uint64", "Uint64", "%s != 0", "protowire.VarintType"},
	"float32": {"float32", "Float32", "%s != 0", "protowire.Fixed32Type"},
	"float64": {"float64", "Float64", "%s != 0", "protowire.Fixed64Type"},
	"string":  {"string", "String", `%s != ""`, ""},
	"bytes":   {"[]byte", "Bytes", "len(%s) > 0", ""},
	"time":    {"time.Time", "Time", "!%s.IsZero()", ""},
}

// Codec returns Go source, without a package clause or imports, that gives every type of s two
// methods for the proto3 wire format with the field numbers of s:
//   - aicodedAppend(b []byte) []byte appends the fields of the value to b. A singular field is left
//     out when it holds its zero value, a non-nil pointer is always written, and a repeated field
//     writes every element.
//   - aicodedDecode(n protowire.Number, v wire.Value) error reads field n into the value, for
//     wire.Decode and wire.Value.Decode. Unknown fields are skipped, repeated scalars are taken
//     packed or not, and a nested struct starts from its zero value each time it arrives.
//
// With declare, the source also declares the types. It uses the packages protowire and wire,
// and time when it declares a type with a time field.
func Codec(s rpcschema.Schema, declare bool) []byte {
	var b bytes.Buffer
	for _, name := range slices.Sorted(maps.Keys(s.Types)) {
		fields := byNumber(s.Types[name])
		if declare {
			fmt.Fprintf(&b, "type %s struct {\n", name)
			for _, f := range fields {
				fmt.Fprintf(&b, "%s %s\n", f.name, goType(f.Type))
			}
			b.WriteString("}\n\n")
		}
		fmt.Fprintf(&b, "func (x *%s) aicodedAppend(b []byte) []byte {\n", name)
		for _, f := range fields {
			appendField(&b, f)
		}
		b.WriteString("return b\n}\n\n")
		if len(fields) == 0 {
			fmt.Fprintf(&b, "func (x *%s) aicodedDecode(protowire.Number, wire.Value) error {\nreturn nil\n}\n\n", name)
			continue
		}
		fmt.Fprintf(&b, "func (x *%s) aicodedDecode(n protowire.Number, v wire.Value) error {\nswitch n {\n", name)
		for _, f := range fields {
			fmt.Fprintf(&b, "case %d:\n", f.Number)
			decodeField(&b, f)
		}
		b.WriteString("}\nreturn nil\n}\n\n")
	}
	return b.Bytes()
}

// namedField is a field of a snapshot type with its name.
type namedField struct {
	name string
	rpcschema.Field
}

// byNumber returns the fields of t sorted by number.
func byNumber(t rpcschema.Type) []namedField {
	out := make([]namedField, 0, len(t.Fields))
	for name, f := range t.Fields {
		out = append(out, namedField{name, f})
	}
	slices.SortFunc(out, func(a, b namedField) int { return cmp.Compare(a.Number, b.Number) })
	return out
}

// goType returns the Go type of the snapshot type t.
func goType(t string) string {
	switch {
	case strings.HasPrefix(t, "[]"):
		return "[]" + goType(t[2:])
	case strings.HasPrefix(t, "*"):
		return "*" + goType(t[1:])
	}
	if sc, ok := wireScalars[t]; ok {
		return sc.goType
	}
	return t
}

func appendField(b *bytes.Buffer, f namedField) {
	x := "x." + f.name
	switch {
	case strings.HasPrefix(f.Type, "[]"):
		if sc, ok := wireScalars[f.Type[2:]]; ok {
			fmt.Fprintf(b, "for _, v := range %s {\nb = wire.Append%s(b, %d, v)\n}\n", x, sc.fn, f.Number)
		} else {
			fmt.Fprintf(b, "for i := range %s {\nb = wire.AppendMessage(b, %d, %s[i].aicodedAppend)\n}\n", x, f.Number, x)
		}
	case strings.HasPrefix(f.Type, "*"):
		if sc, ok := wireScalars[f.Type[1:]]; ok {
			fmt.Fprintf(b, "if %s != nil {\nb = wire.Append%s(b, %d, *%s)\n}\n", x, sc.fn, f.Number, x)
		} else {
			fmt.Fprintf(b, "if %s != nil {\nb = wire.AppendMessage(b, %d, %s.aicodedAppend)\n}\n", x, f.Number, x)
		}
	default:
		if sc, ok := wireScalars[f.Type]; ok {
			fmt.Fprintf(b, "if %s {\nb = wire.Append%s(b, %d, %s)\n}\n", fmt.Sprintf(sc.nonZero, x), sc.fn, f.Number, x)
		} else {
			fmt.Fprintf(b, "if m := %s.aicodedAppend(nil); len(m) > 0 {\nb = wire.AppendBytes(b, %d, m)\n}\n", x, f.Number)
		}
	}
}

func decodeField(b *bytes.Buffer, f namedField) {
	x := "x." + f.name
	switch {
	case strings.HasPrefix(f.Type, "[]"):
		elem := f.Type[2:]
		sc, ok := wireScalars[elem]
		switch {
		case ok && sc.packed != "":
			fmt.Fprintf(b, "return v.Packed(%s, func(v wire.Value) error {\ne, err := v.%s()\nif err != nil {\nreturn err\n}\n%s = append(%s, e)\nreturn nil\n})\n", sc.packed, sc.fn, x, x)
		case ok:
			fmt.Fprintf(b, "e, err := v.%s()\nif err != nil {\nreturn err\n}\n%s = append(%s, e)\n", sc.fn, x, x)
		default:
			fmt.Fprintf(b, "var e %s\nif err := v.Decode(e.aicodedDecode); err != nil {\nreturn err\n}\n%s = append(%s, e)\n", elem, x, x)
		}
	case strings.HasPrefix(f.Type, "*"):
		elem := f.Type[1:]
		if sc, ok := wireScalars[elem]; ok {
			fmt.Fprintf(b, "e, err := v.%s()\nif err != nil {\nreturn err\n}\n%s = &e\n", sc.fn, x)
		} else {
			fmt.Fprintf(b, "%s = new(%s)\nreturn v.Decode(%s.aicodedDecode)\n", x, elem, x)
		}
	default:
		if sc, ok := wireScalars[f.Type]; ok {
			fmt.Fprintf(b, "var err error\n%s, err = v.%s()\nreturn err\n", x, sc.fn)
		} else {
			fmt.Fprintf(b, "%s = %s{}\nreturn v.Decode(%s.aicodedDecode)\n", x, f.Type, x)
		}
	}
}
