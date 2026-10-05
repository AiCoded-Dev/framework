package rpcschema

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
)

// Break is one change in a schema that breaks calls made with an older snapshot of it. Path is
// the field, such as GetInvoiceIn.Lines[].Amount; it is empty for a removed function.
type Break struct {
	Method, Path, Reason string
}

// Compatible returns the changes in current that break calls made with the snapshot held: a
// removed function, and any field reachable from a function's input or output that was removed,
// renamed, renumbered or given another type. Added functions and fields break nothing. Type
// names may change. The breaks are sorted by function, then path.
func Compatible(held, current Schema) []Break {
	var out []Break
	for _, name := range slices.Sorted(maps.Keys(held.Methods)) {
		hm := held.Methods[name]
		cm, ok := current.Methods[name]
		if !ok {
			out = append(out, Break{Method: name, Reason: "function removed"})
			continue
		}
		w := &walker{held: held, current: current, method: name, seen: map[[2]string]bool{}}
		w.compare(hm.In, cm.In, hm.In)
		w.compare(hm.Out, cm.Out, hm.Out)
		out = append(out, w.breaks...)
	}
	slices.SortFunc(out, func(a, b Break) int {
		return cmp.Or(cmp.Compare(a.Method, b.Method), cmp.Compare(a.Path, b.Path))
	})
	return out
}

type walker struct {
	held, current Schema
	method        string
	seen          map[[2]string]bool
	breaks        []Break
}

func (w *walker) add(path, format string, args ...any) {
	w.breaks = append(w.breaks, Break{Method: w.method, Path: path, Reason: fmt.Sprintf(format, args...)})
}

// compare compares the held type h with the current type c, both reached at path. It pairs the
// fields by name and number.
func (w *walker) compare(h, c, path string) {
	if w.seen[[2]string{h, c}] {
		return
	}
	w.seen[[2]string{h, c}] = true
	ht, ct := w.held.Types[h], w.current.Types[c]
	for _, name := range slices.Sorted(maps.Keys(ht.Fields)) {
		hf, p := ht.Fields[name], path+"."+name
		cf, ok := ct.Fields[name]
		switch {
		case ok && cf.Number == hf.Number:
			w.field(hf.Type, cf.Type, p)
		case ok:
			w.add(p, "number changed from %d to %d", hf.Number, cf.Number)
		case named(ct, hf.Number) != "":
			w.add(p, "renamed to %s", named(ct, hf.Number))
		default:
			w.add(p, "removed")
		}
	}
}

// field compares the type h of a held field with the type c of the current one.
func (w *walker) field(h, c, path string) {
	hp, he, _ := shape(h)
	cp, ce, _ := shape(c)
	switch {
	case hp != cp || scalars[he] != scalars[ce] || (scalars[he] && he != ce):
		w.add(path, "type changed from %s to %s", h, c)
	case !scalars[he]:
		if hp == "[]" {
			path += "[]"
		}
		w.compare(he, ce, path)
	}
}

// named returns the name of t's field with number n, or "".
func named(t Type, n int) string {
	for name, f := range t.Fields {
		if f.Number == n {
			return name
		}
	}
	return ""
}
