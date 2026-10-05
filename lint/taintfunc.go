package lint

import (
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// flow is the analysis of one function: what each of its values may carry, at any point of the
// function, given the summaries of the functions it calls.
type flow struct {
	t  *taint
	fn *ssa.Function
	// l holds what each value may carry. A value that refers to memory carries what that
	// memory holds.
	l map[ssa.Value]labels
	// sum is the summary of fn that the analysis builds.
	sum *summary
	// globals holds the global variables fn uses.
	globals []*ssa.Global
	// changed says that a pass over fn changed l.
	changed bool
}

// analyze returns the summary of fn, given the summaries and places found so far. It records in
// t.found the outside data that reaches a recording call of fn, and in t.places what fn puts in
// a global variable or a pool.
func (t *taint) analyze(fn *ssa.Function) *summary {
	f := &flow{t: t, fn: fn, l: map[ssa.Value]labels{}, sum: &summary{
		results: make([]labels, fn.Signature.Results().Len()),
		inputs:  make([]labels, len(fn.Params)+len(fn.FreeVars)),
		sinks:   map[sink]labels{}, places: map[place]labels{},
	}}
	hooks := t.hook(fn)
	pooled := t.places[place{pool: contextPool}]
	for i, v := range f.inputs() {
		l := input(i)
		if i < len(hooks) {
			l |= hooks[i]
		}
		if isRequest(v.Type()) {
			l |= request
		}
		if t.contextual(v.Type()) {
			l |= pooled
		}
		f.add(v, l)
	}
	for _, b := range fn.Blocks {
		for _, instr := range b.Instrs {
			if v, ok := instr.(ssa.Value); ok {
				if isRequest(v.Type()) {
					f.add(v, request)
				}
				if t.contextual(v.Type()) {
					f.add(v, pooled)
				}
			}
			for _, op := range instr.Operands(nil) {
				if g, ok := (*op).(*ssa.Global); ok && !slices.Contains(f.globals, g) {
					f.globals = append(f.globals, g)
					f.add(g, t.places[place{global: g}])
				}
			}
		}
	}
	keeps := t.keeps(fn)
	if keeps {
		f.add(fn.Params[0], t.places[place{pool: statePool}])
	}
	for f.changed = true; f.changed; {
		f.changed = false
		for _, b := range fn.Blocks {
			for _, instr := range b.Instrs {
				f.step(instr)
			}
		}
	}
	if keeps {
		f.put(place{pool: statePool}, f.l[fn.Params[0]]&kinds)
	}
	for i, v := range f.inputs() {
		if t.refers(v.Type()) {
			f.sum.inputs[i] = f.l[v] &^ input(i)
		}
	}
	for _, g := range f.globals {
		f.put(place{global: g}, f.l[g])
	}
	for v, l := range f.l {
		if t.contextual(v.Type()) {
			f.put(place{pool: contextPool}, l)
		}
	}
	return f.sum
}

// inputs returns the inputs of fn: its parameters, then its free variables.
func (f *flow) inputs() []ssa.Value {
	var in []ssa.Value
	for _, p := range f.fn.Params {
		in = append(in, p)
	}
	for _, v := range f.fn.FreeVars {
		in = append(in, v)
	}
	return in
}

// step applies instr to what the values of fn carry.
func (f *flow) step(instr ssa.Instruction) {
	switch i := instr.(type) {
	case *ssa.Phi:
		for _, e := range i.Edges {
			f.link(e, i)
		}
	case *ssa.BinOp:
		f.flow(i.X, i)
		f.flow(i.Y, i)
	case *ssa.UnOp:
		f.link(i.X, i)
	case *ssa.Convert:
		f.flow(i.X, i)
	case *ssa.MultiConvert:
		f.flow(i.X, i)
	case *ssa.ChangeType:
		f.link(i.X, i)
	case *ssa.ChangeInterface:
		f.link(i.X, i)
		f.outOfContext(i.X, i, i.Type())
	case *ssa.SliceToArrayPointer:
		f.link(i.X, i)
	case *ssa.MakeInterface:
		f.link(i.X, i)
		f.implicit(i)
	case *ssa.TypeAssert:
		f.link(i.X, i)
		f.outOfContext(i.X, i, i.AssertedType)
	case *ssa.Slice:
		f.link(i.X, i)
	case *ssa.FieldAddr:
		f.link(i.X, i)
	case *ssa.Field:
		f.link(i.X, i)
	case *ssa.IndexAddr:
		f.link(i.X, i)
	case *ssa.Index:
		f.link(i.X, i)
	case *ssa.Lookup:
		f.link(i.X, i)
	case *ssa.Next:
		f.flow(i.Iter.(*ssa.Range).X, i)
	case *ssa.Extract:
		if _, ok := i.Tuple.(*ssa.Call); !ok {
			f.flow(i.Tuple, i)
		}
	case *ssa.Select:
		for _, s := range i.States {
			if s.Dir == types.RecvOnly {
				f.flow(s.Chan, i)
			} else {
				f.link(s.Send, s.Chan)
			}
		}
	case *ssa.MakeClosure:
		for _, b := range i.Bindings {
			f.link(b, i)
		}
		f.closure(i)
	case *ssa.Store:
		f.link(i.Val, i.Addr)
	case *ssa.MapUpdate:
		f.flow(i.Key, i.Map)
		f.link(i.Value, i.Map)
	case *ssa.Send:
		f.link(i.X, i.Chan)
	case *ssa.Return:
		for j, r := range i.Results {
			f.sum.results[j] |= f.l[r]
		}
	case *ssa.Panic:
		f.put(place{pool: panicPool}, f.l[i.X])
	case *ssa.Call:
		f.call(i.Common(), i)
	case *ssa.Go:
		f.call(i.Common(), nil)
	case *ssa.Defer:
		f.call(i.Common(), nil)
	}
}

// outOfContext gives v, the conversion of x into type typ, what the context pool holds when x is
// a context and v is not, such as a context converted to any or asserted to another type.
func (f *flow) outOfContext(x, v ssa.Value, typ types.Type) {
	if isContext(x.Type()) && !isContext(typ) {
		f.add(v, f.t.places[place{pool: contextPool}])
	}
}

// add adds l to what v carries, as far as v's type can carry it.
func (f *flow) add(v ssa.Value, l labels) {
	switch v.(type) {
	case nil, *ssa.Const, *ssa.Function, *ssa.Builtin:
		return
	}
	if l == 0 || l&^f.l[v] == 0 || !f.t.carries(v.Type()) {
		return
	}
	f.l[v] |= l
	f.changed = true
}

// flow adds what from carries to to.
func (f *flow) flow(from, to ssa.Value) {
	f.add(to, f.l[from])
}

// link adds what from carries to to, and, when both refer to memory they may share, what to
// carries to from.
func (f *flow) link(from, to ssa.Value) {
	f.flow(from, to)
	if f.t.refers(from.Type()) && f.t.refers(to.Type()) {
		f.flow(to, from)
	}
}

// put adds l to what the place p holds: its kinds of outside data for every caller, and its
// inputs to the summary.
func (f *flow) put(p place, l labels) {
	if k := l & kinds; k&^f.t.places[p] != 0 {
		f.t.places[p] |= k
		f.t.changed = true
	}
	if in := l &^ kinds; in != 0 {
		f.sum.places[p] |= in
	}
}

// reach records that l reaches the recording call at: its kinds of outside data as a finding,
// and its inputs in the summary.
func (f *flow) reach(at sink, l labels) {
	if k := l & kinds; k != 0 && at.pos.IsValid() {
		f.t.found[at] |= k
	}
	if in := l &^ kinds; in != 0 {
		f.sum.sinks[at] |= in
	}
}

// record reports that the call at pos of what messages name name records the values args.
func (f *flow) record(pos token.Pos, name string, args []ssa.Value) {
	var l labels
	for _, a := range args {
		l |= f.l[a]
	}
	f.reach(sink{pos: f.t.checked(pos), name: name}, l)
}

// hook returns what the parameters of fn are when fn is a function the framework calls with
// outside data: a function the app serves to other apps, a page call, the Validate hook of a
// live value, or a method that reads a value from the database as a sql.Scanner.
func (t *taint) hook(fn *ssa.Function) []labels {
	sig := fn.Signature
	params := make([]labels, len(fn.Params))
	switch name := fn.Name(); {
	case sig.Recv() == nil && fn.Pkg != nil && fn.Pkg.Pkg.Path() == t.rpc && token.IsExported(name) && fn.Parent() == nil:
		for i := range params {
			params[i] = rpcInput
		}
	case sig.Recv() == nil:
		return nil
	case len(name) > len("Call") && strings.HasPrefix(name, "Call") && isHook(sig, false):
		params[3] = callInput
	case len(name) > len("Validate") && strings.HasPrefix(name, "Validate") && isHook(sig, true):
		params[3] = liveValue
	case name == "Scan" && sig.Params().Len() == 1 && sig.Results().Len() == 1 &&
		types.Identical(sig.Params().At(0).Type(), types.NewInterfaceType(nil, nil)) &&
		types.Identical(sig.Results().At(0).Type(), types.Universe.Lookup("error").Type()):
		params[1] = dbValue
	default:
		return nil
	}
	return params
}

// keeps reports whether fn is a hook of a page, a method of a hand-written file that the
// framework calls with the request: its receiver keeps what it holds between requests.
func (t *taint) keeps(fn *ssa.Function) bool {
	if fn.Signature.Recv() == nil || fn.Synthetic != "" || !t.handWritten(fn.Pos()) {
		return false
	}
	for p := range fn.Signature.Params().Variables() {
		if isRequest(p.Type()) {
			return true
		}
	}
	return false
}

// isHook reports whether sig is the signature of a page call, func(ctx context.Context, r
// *web.Request, in In) (Out, error), or with same, of a Validate hook, whose Out is In.
func isHook(sig *types.Signature, same bool) bool {
	p, r := sig.Params(), sig.Results()
	return p.Len() == 3 && r.Len() == 2 && named(p.At(0).Type()) == "context.Context" &&
		isRequest(p.At(1).Type()) && types.Identical(r.At(1).Type(), types.Universe.Lookup("error").Type()) &&
		(!same || types.Identical(p.At(2).Type(), r.At(0).Type()))
}
