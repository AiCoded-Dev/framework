package lint

import (
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// implicitMethods are the methods that fmt, log/slog and encoding/json call on the values they
// format and that return text, and jsonMethods those of them that encoding/json calls.
var (
	implicitMethods = []string{"Error", "GoString", "LogValue", "MarshalJSON", "MarshalText", "String"}
	jsonMethods     = []string{"MarshalJSON", "MarshalText"}
)

// call applies c, a call whose result is v, or nil for go and defer, to what the values carry.
func (f *flow) call(c *ssa.CallCommon, v ssa.Value) {
	out := results(v, c.Signature().Results().Len())
	if b, ok := c.Value.(*ssa.Builtin); ok {
		f.builtin(b.Name(), c, v)
		return
	}
	args := c.Args
	if c.IsInvoke() {
		args = append([]ssa.Value{c.Value}, c.Args...)
	}
	switch g := c.StaticCallee(); {
	case g != nil && g.Blocks != nil:
		f.enter(g, c.Value, args, out, c.Pos())
	case g != nil:
		f.outside(calleeName(g), nil, args, out, c.Pos())
	case c.IsInvoke():
		for _, m := range f.t.implementations(c) {
			f.enter(m, nil, args, out, c.Pos())
		}
		f.outside(f.t.interfaceMember(c.Method), nil, args, out, c.Pos())
	default:
		if types.Identical(c.Signature(), causeFunc) {
			f.put(place{pool: contextPool}, f.l[c.Args[0]])
		}
		fns, _ := f.t.bySig.At(c.Signature()).([]*ssa.Function)
		for _, g := range fns {
			f.enter(g, c.Value, args, out, c.Pos())
		}
		outside, _ := f.t.outsideBySig.At(c.Signature()).([]*ssa.Function)
		for _, g := range outside {
			f.outside(calleeName(g), nil, args, out, c.Pos())
		}
		f.outside(named(c.Value.Type()), c.Value, args, out, c.Pos())
	}
}

// results returns, for each of the n results of the call v, the values that hold it: v itself,
// or the values that extract it from v's tuple. It returns n empty lists when v is nil.
func results(v ssa.Value, n int) [][]ssa.Value {
	out := make([][]ssa.Value, n)
	switch {
	case v == nil:
	case n == 1:
		out[0] = []ssa.Value{v}
	default:
		for _, r := range *v.Referrers() {
			if ex, ok := r.(*ssa.Extract); ok {
				out[ex.Index] = append(out[ex.Index], ex)
			}
		}
	}
	return out
}

// builtin applies a call of the builtin function name, whose result is v.
func (f *flow) builtin(name string, c *ssa.CallCommon, v ssa.Value) {
	switch name {
	case "print", "println":
		f.record(c.Pos(), name, c.Args)
	case "copy":
		f.flow(c.Args[1], c.Args[0])
	case "append":
		for _, a := range c.Args {
			f.link(a, v)
		}
	case "recover":
		f.add(v, f.t.places[place{pool: panicPool}])
	case "ssa:wrapnilchk":
		f.link(c.Args[0], v)
	default:
		for _, a := range c.Args {
			f.flow(a, v)
		}
	}
}

// enter applies a call at pos of g, a function of the app, through the function value fv, with
// args.
func (f *flow) enter(g *ssa.Function, fv ssa.Value, args []ssa.Value, out [][]ssa.Value, pos token.Pos) {
	vals := inputsOf(g, fv, args)
	f.apply(g, vals, f.labelsOf(vals), out, pos)
}

// inputsOf returns the value that each input of g is when fv, a function value that may be g, is
// called with args: args for the parameters, and for the free variables the bindings of fv, a
// closure of g, or else fv itself.
func inputsOf(g *ssa.Function, fv ssa.Value, args []ssa.Value) []ssa.Value {
	vals := make([]ssa.Value, len(g.Params)+len(g.FreeVars))
	copy(vals, args)
	for i := range g.FreeVars {
		if mc, ok := fv.(*ssa.MakeClosure); ok && mc.Fn == g {
			vals[len(g.Params)+i] = mc.Bindings[i]
		} else {
			vals[len(g.Params)+i] = fv
		}
	}
	return vals
}

// labelsOf returns what each of vals carries.
func (f *flow) labelsOf(vals []ssa.Value) []labels {
	in := make([]labels, len(vals))
	for i, v := range vals {
		in[i] = f.l[v]
	}
	return in
}

// apply applies the summary of g, a function of the app, to a call at pos. in holds what each
// input of g carries, and vals the value each input is, for the call to write into, or nil.
// out holds, for each result of g, the values that get it.
func (f *flow) apply(g *ssa.Function, vals []ssa.Value, in []labels, out [][]ssa.Value, pos token.Pos) {
	s := f.t.sums[g]
	if s == nil {
		return
	}
	conv := func(l labels) labels {
		r := l & kinds
		for i, il := range in {
			if l&input(i) != 0 {
				r |= il
			}
		}
		return r
	}
	for j, r := range s.results {
		for _, o := range out[j] {
			f.add(o, conv(r))
			for i, v := range vals {
				if v != nil && r&input(i) != 0 && f.t.refers(o.Type()) && f.t.refers(v.Type()) {
					f.flow(o, v)
				}
			}
		}
	}
	for i, w := range s.inputs {
		if vals[i] != nil {
			f.add(vals[i], conv(w))
		}
	}
	for at, l := range s.sinks {
		if !at.pos.IsValid() {
			at.pos = f.t.checked(pos)
		}
		f.reach(at, conv(l))
	}
	for p, l := range s.places {
		f.put(p, conv(l))
	}
}

// outside applies a call at pos of name, a function or method outside the app as the tables
// name it, the named type of the function value called, or "" for one that may be any, with
// args, the receiver first. fv is the function value called, or nil: its results also carry what
// fv carries.
func (f *flow) outside(name string, fv ssa.Value, args []ssa.Value, out [][]ssa.Value, pos token.Pos) {
	first := 0
	if isMethod(name) && len(args) > 0 {
		first = 1
	}
	all, in := f.l[fv], f.l[fv]
	for i, a := range args {
		all |= f.l[a]
		if i >= first {
			in |= f.l[a]
		}
	}
	if recorders[name] {
		f.record(pos, display(name), args[first:])
	}
	if k, ok := sources[name]; ok {
		all = k | in
	}
	if p, ok := getters[name]; ok {
		all |= f.t.places[place{pool: p}]
	}
	p := putters[name]
	for _, i := range p.args {
		if i < len(args) {
			f.put(place{pool: p.pool}, f.l[args[i]])
		}
	}
	for _, i := range writers[name] {
		if i < len(args) {
			f.add(args[i], all)
		}
	}
	for _, vs := range out {
		for _, o := range vs {
			f.add(o, all)
			if i, ok := keepers[name]; ok && i < len(args) {
				f.link(o, args[i])
			}
		}
	}
	if pair, ok := links[name]; ok {
		for _, a := range operands(args, out, pair[0]) {
			for _, b := range operands(args, out, pair[1]) {
				f.link(a, b)
			}
		}
	}
	f.callbacks(args, all, slices.Concat(out...), pos)
}

// operands returns the values that operand i of a call with args and out is: argument i, or the
// values that get result i-len(args).
func operands(args []ssa.Value, out [][]ssa.Value, i int) []ssa.Value {
	if i < len(args) {
		return []ssa.Value{args[i]}
	}
	if i -= len(args); i < len(out) {
		return out[i]
	}
	return nil
}

// callbacks applies the calls that a function outside the app may make of the functions of the
// app among args: each gets in for each parameter, and its results reach out.
func (f *flow) callbacks(args []ssa.Value, in labels, out []ssa.Value, pos token.Pos) {
	for _, a := range args {
		if _, ok := a.Type().Underlying().(*types.Signature); !ok {
			continue
		}
		for _, g := range f.t.values(a) {
			vals := inputsOf(g, a, nil)
			ins := f.labelsOf(vals)
			for i := range g.Params {
				ins[i] = in
			}
			res := make([][]ssa.Value, g.Signature.Results().Len())
			for j := range res {
				res[j] = out
			}
			f.apply(g, vals, ins, res, pos)
		}
	}
}

// closure applies the closure mc as if it were called with no outside data in its parameters,
// so that what its free variables carry reaches what it records and writes, wherever it is
// called.
func (f *flow) closure(mc *ssa.MakeClosure) {
	g := mc.Fn.(*ssa.Function)
	f.enter(g, mc, nil, make([][]ssa.Value, g.Signature.Results().Len()), mc.Pos())
}

// implicit applies what fmt, log/slog and encoding/json reach when they format mi's value: what
// a context it holds carries, what the methods of the app they call on it, or on what it holds,
// return, and what its Format methods write into their fmt.State reach the interface value.
func (f *flow) implicit(mi *ssa.MakeInterface) {
	s := f.t.shows(mi.X.Type())
	if s.context {
		f.add(mi, f.t.places[place{pool: contextPool}])
	}
	for _, g := range s.methods {
		out := make([][]ssa.Value, g.Signature.Results().Len())
		for j := range out {
			out[j] = []ssa.Value{mi}
		}
		f.enter(g, g, []ssa.Value{mi.X}, out, mi.Pos())
	}
	for _, g := range s.formats {
		f.enter(g, g, []ssa.Value{mi.X, mi}, nil, mi.Pos())
	}
}

// values returns the functions of the app that the function value v may be.
func (t *taint) values(v ssa.Value) []*ssa.Function {
	switch v := v.(type) {
	case *ssa.MakeClosure:
		return []*ssa.Function{v.Fn.(*ssa.Function)}
	case *ssa.Function:
		if v.Blocks != nil {
			return []*ssa.Function{v}
		}
		return nil
	}
	fns, _ := t.bySig.At(v.Type().Underlying()).([]*ssa.Function)
	return fns
}

// implementations returns the methods of the app that c, a call of an interface method, may
// call: those of its name whose receiver implements the interface.
func (t *taint) implementations(c *ssa.CallCommon) []*ssa.Function {
	key := imethod{c.Value.Type().Underlying().(*types.Interface), c.Method.Id()}
	fns, ok := t.implements[key]
	if !ok {
		for _, fn := range t.byID[key.id] {
			if types.Implements(fn.Signature.Recv().Type(), key.iface) {
				fns = append(fns, fn)
			}
		}
		t.implements[key] = fns
	}
	return fns
}

// interfaceMember returns how the tables name m, a method of an interface: as itself, or as the
// method of the tables that it stands for, with its name and signature.
func (t *taint) interfaceMember(m *types.Func) string {
	if m.Pkg() == nil {
		return "" // error.Error
	}
	name := member(m)
	if tabled[name] {
		return name
	}
	if c := standsFor(m, t.methods); c != nil {
		return member(c)
	}
	return name
}

// isMethod reports whether name, as the tables name a function or method, names a method.
func isMethod(name string) bool {
	if name == "" {
		return false
	}
	_, rest := splitMember(name)
	return strings.Contains(rest, ".")
}

// calleeName returns how the tables name g, a function outside the app, or "".
func calleeName(g *ssa.Function) string {
	if o := g.Origin(); o != nil {
		g = o
	}
	if obj, ok := g.Object().(*types.Func); ok && obj.Pkg() != nil {
		return member(obj)
	}
	return ""
}
