package rpcschema

import (
	"maps"
	"slices"

	"google.golang.org/protobuf/encoding/protowire"
)

// Number returns fresh with its field numbers set; fresh's own numbers are ignored. A field
// keeps the number prev gives it under the same type and field name. A new field gets one more
// than the highest number its type has ever used. The number of a field prev has and fresh does
// not is retired for good. Types no function reaches are left out. prev may be nil.
func Number(prev *Schema, fresh Schema) Schema {
	out := Schema{App: fresh.App, Methods: maps.Clone(fresh.Methods), Types: map[string]Type{}}
	if out.Methods == nil {
		out.Methods = map[string]Method{}
	}
	for name := range reachable(fresh) {
		var old Type
		if prev != nil {
			old = prev.Types[name]
		}
		out.Types[name] = numberType(old, fresh.Types[name])
	}
	return out
}

func numberType(old, fresh Type) Type {
	t := Type{Fields: make(map[string]Field, len(fresh.Fields)), Retired: slices.Clone(old.Retired)}
	next := 0
	for _, n := range old.Retired {
		next = max(next, n)
	}
	for _, f := range old.Fields {
		next = max(next, f.Number)
	}
	for _, name := range slices.Sorted(maps.Keys(fresh.Fields)) {
		f := fresh.Fields[name]
		if o, ok := old.Fields[name]; ok {
			f.Number = o.Number
		} else {
			next++
			if next >= int(protowire.FirstReservedNumber) && next <= int(protowire.LastReservedNumber) {
				next = int(protowire.LastReservedNumber) + 1
			}
			f.Number = next
		}
		t.Fields[name] = f
	}
	for name, o := range old.Fields {
		if _, ok := fresh.Fields[name]; !ok {
			t.Retired = append(t.Retired, o.Number)
		}
	}
	slices.Sort(t.Retired)
	return t
}

// reachable returns the names of the types s's functions reach.
func reachable(s Schema) map[string]bool {
	seen := map[string]bool{}
	var visit func(name string)
	visit = func(name string) {
		t, ok := s.Types[name]
		if !ok || seen[name] {
			return
		}
		seen[name] = true
		for _, f := range t.Fields {
			if _, elem, _ := shape(f.Type); !scalars[elem] {
				visit(elem)
			}
		}
	}
	for _, m := range s.Methods {
		visit(m.In)
		visit(m.Out)
	}
	return seen
}
