package lint

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"go/token"
	"go/types"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
	"golang.org/x/tools/go/types/typeutil"
)

// labels is what a value may carry, one bit each: the kinds of outside data, and, while a
// function is analyzed, the inputs of the function it comes from.
type labels uint64

// The kinds of outside data, in the order a message names them.
const (
	formValue labels = 1 << iota
	urlParam
	request
	viewer
	dbValue
	inboxMail
	storedFile
	rpcInput
	callInput
	liveValue
	appAnswer
	secret
)

// kindNames names the kinds of outside data, in the order of their bits.
var kindNames = []string{
	"a form value", "a URL parameter", "the request", "the viewer's identity", "a value from the database",
	"a mail from the inbox", "a stored file", "the input of an RPC function", "the input of a page call",
	"a live value from the browser", "the answer of another app", "a secret",
}

const (
	// inputShift is the bit of a function's first input; the bits below it are kinds.
	inputShift = 16
	// kinds holds every kind of outside data.
	kinds labels = 1<<inputShift - 1
	// lastInput is the last input with a bit of its own; the inputs after it share its bit.
	lastInput = 63 - inputShift
)

// input returns the bit of input i of a function: a parameter, or a free variable after the
// parameters.
func input(i int) labels { return 1 << (inputShift + min(i, lastInput)) }

// describe names the kinds of outside data in l: "a form value or a URL parameter".
func describe(l labels) string {
	var names []string
	for i, name := range kindNames {
		if l&(1<<i) != 0 {
			names = append(names, name)
		}
	}
	return either(names)
}

// either joins names with commas and a final "or".
func either(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]
}

// summary is what a function of the app does with its inputs, as far as the analysis knows yet.
type summary struct {
	// results holds what each result carries.
	results []labels
	// inputs holds what the function writes into the memory each input refers to.
	inputs []labels
	// sinks holds the inputs that reach each recording call.
	sinks map[sink]labels
	// places holds the inputs that flow into each global variable and pool.
	places map[place]labels
}

// join adds what o holds to s.
func (s *summary) join(o *summary) {
	for j, r := range o.results {
		s.results[j] |= r
	}
	for i, w := range o.inputs {
		s.inputs[i] |= w
	}
	for at, l := range o.sinks {
		s.sinks[at] |= l
	}
	for p, l := range o.places {
		s.places[p] |= l
	}
}

// equal reports whether s and o hold the same.
func (s *summary) equal(o *summary) bool {
	return o != nil && slices.Equal(s.results, o.results) && slices.Equal(s.inputs, o.inputs) &&
		maps.Equal(s.sinks, o.sinks) && maps.Equal(s.places, o.places)
}

// sink is a call that records a value: its position, and what it calls, as messages name it.
type sink struct {
	pos  token.Pos
	name string
}

// place is a global variable or a pool.
type place struct {
	global *ssa.Global
	pool   pool
}

// taint is one run of E-LINT-008 over an app.
type taint struct {
	a    *app
	prog *ssa.Program
	// funcs are the functions with a body: the app's own, their instances and the wrappers
	// around methods, in the order of their names.
	funcs []*ssa.Function
	// sums holds the summary of each function of funcs found so far.
	sums map[*ssa.Function]*summary
	// places holds the kinds of outside data that each global variable and pool holds so far.
	places map[place]labels
	// found holds the kinds of outside data that reach each recording call of the app's files.
	found map[sink]labels
	// changed says that a round changed a summary or a place.
	changed bool
	// rpc is the import path of the app's rpc/ package.
	rpc string
	// methods holds the methods that the tables name, by name.
	methods map[string][]*types.Func
	// bySig holds the functions of funcs that a dynamic call may call: those without a
	// receiver, by signature. outsideBySig holds the functions outside the app that the app
	// uses as values.
	bySig, outsideBySig typeutil.Map
	// byID holds the methods of funcs by id, for calls of interface methods.
	byID map[string][]*ssa.Function
	// implements caches which methods of byID an interface method may call.
	implements map[imethod][]*ssa.Function
	// carry and ref cache whether values of a type carry outside data and refer to memory.
	carry, ref map[types.Type]bool
	// ctx is the interface of context.Context, or nil when no package of the program uses one.
	ctx *types.Interface
	// ctxTypes caches whether a type, or the pointer to it, implements ctx.
	ctxTypes map[types.Type]bool
	// shown caches what formatting a value of a type reaches.
	shown map[types.Type]*shown
}

// shown is what fmt, log/slog and encoding/json reach when they format a value of a type: the
// value itself, and what it holds through pointers, the elements of slices, arrays and maps,
// exported fields and embedded fields. context says that they reach a context, which prints its
// values; methods are the methods of the app that they call and that return text, such as
// String; formats are the Format methods that fmt calls, which write text into their fmt.State.
type shown struct {
	context bool
	methods []*ssa.Function
	formats []*ssa.Function
}

// showing is a type that show walks, and whether it walks it for encoding/json alone.
type showing struct {
	typ  types.Type
	json bool
}

// imethod is a method of an interface.
type imethod struct {
	iface *types.Interface
	id    string
}

// taint reports, with E-LINT-008, outside data that reaches a recording call in a file of the
// app, generated files included: a log, a print or a span. A call that may be one of several
// recording functions, such as a call of a function value, is reported once and names them all.
// Its error says that ctx ended, that no package belongs to the app's module, or that a package
// cannot be built for the analysis.
func (a *app) taint(ctx context.Context, pkgs []*packages.Package) error {
	var rpc string
	for _, p := range pkgs {
		if p.Module != nil && p.Module.Main {
			rpc = p.Module.Path + "/rpc"
		}
	}
	if rpc == "" {
		return errors.New("lint cannot trace outside data (E-LINT-008): no package belongs to the app's module")
	}
	prog, built := ssautil.Packages(pkgs, ssa.InstantiateGenerics)
	if i := slices.Index(built, nil); i >= 0 {
		return fmt.Errorf("lint cannot trace outside data (E-LINT-008): package %s cannot be built", pkgs[i].PkgPath)
	}
	prog.Build()
	t := &taint{
		a: a, prog: prog, sums: map[*ssa.Function]*summary{}, places: map[place]labels{}, found: map[sink]labels{},
		rpc: rpc, methods: methodsOf(pkgs, tableMethods()), byID: map[string][]*ssa.Function{},
		implements: map[imethod][]*ssa.Function{}, carry: map[types.Type]bool{}, ref: map[types.Type]bool{},
		ctxTypes: map[types.Type]bool{}, shown: map[types.Type]*shown{},
	}
	if p := prog.ImportedPackage("context"); p != nil {
		if tn, ok := p.Pkg.Scope().Lookup("Context").(*types.TypeName); ok {
			t.ctx, _ = tn.Type().Underlying().(*types.Interface)
		}
	}
	t.collect()
	for t.changed = true; t.changed; {
		if err := ctx.Err(); err != nil {
			return err
		}
		t.changed = false
		for _, fn := range t.funcs {
			s := t.analyze(fn)
			if old := t.sums[fn]; old != nil {
				s.join(old)
			}
			if !s.equal(t.sums[fn]) {
				t.sums[fn] = s
				t.changed = true
			}
		}
	}
	reached, names := map[token.Pos]labels{}, map[token.Pos][]string{}
	for at, l := range t.found {
		reached[at.pos] |= l
		names[at.pos] = append(names[at.pos], at.name)
	}
	for pos, l := range reached {
		slices.Sort(names[pos])
		file, line, column := a.position(prog.Fset, pos)
		a.report(file, line, column, "E-LINT-008", describe(l)+" reaches "+either(names[pos]))
	}
	return nil
}

// collect fills funcs, and the functions that dynamic calls and calls of interface methods may
// call.
func (t *taint) collect() {
	for fn := range ssautil.AllFunctions(t.prog) {
		if fn.Blocks != nil {
			t.funcs = append(t.funcs, fn)
		}
	}
	slices.SortFunc(t.funcs, func(x, y *ssa.Function) int {
		return cmp.Or(cmp.Compare(x.String(), y.String()), cmp.Compare(x.Pos(), y.Pos()))
	})
	for _, fn := range t.funcs {
		if fn.TypeParams().Len() > len(fn.TypeArgs()) {
			continue // a generic function: its instances are called
		}
		if fn.Signature.Recv() == nil {
			if fn.Synthetic != "package initializer" {
				add(&t.bySig, fn.Signature, fn)
			}
		} else if obj, ok := fn.Object().(*types.Func); ok {
			t.byID[obj.Id()] = append(t.byID[obj.Id()], fn)
		}
		for _, b := range fn.Blocks {
			for _, instr := range b.Instrs {
				t.usedAsValues(instr)
			}
		}
	}
}

// usedAsValues adds the functions outside the app that instr uses as values to outsideBySig.
func (t *taint) usedAsValues(instr ssa.Instruction) {
	var callee ssa.Value
	if c, ok := instr.(ssa.CallInstruction); ok {
		callee = c.Common().Value
	}
	for _, op := range instr.Operands(nil) {
		if g, ok := (*op).(*ssa.Function); ok && g.Blocks == nil && *op != callee {
			if fns, _ := t.outsideBySig.At(g.Signature).([]*ssa.Function); !slices.Contains(fns, g) {
				add(&t.outsideBySig, g.Signature, g)
			}
		}
	}
}

// add adds fn to the functions m holds for sig.
func add(m *typeutil.Map, sig *types.Signature, fn *ssa.Function) {
	fns, _ := m.At(sig).([]*ssa.Function)
	m.Set(sig, append(fns, fn))
}

// carries reports whether a value of type typ can carry outside data: it is, or holds, text, an
// error or another value that can hold text, and it is not a handle.
func (t *taint) carries(typ types.Type) bool {
	c, ok := t.carry[typ]
	if !ok {
		c = holds(typ, isText, map[types.Type]bool{})
		t.carry[typ] = c
	}
	return c
}

// refers reports whether a value of type typ refers to memory that other values can share, or
// holds such a value.
func (t *taint) refers(typ types.Type) bool {
	r, ok := t.ref[typ]
	if !ok {
		r = holds(typ, isRef, map[types.Type]bool{})
		t.ref[typ] = r
	}
	return r
}

// contextual reports whether typ, or the pointer to it, implements context.Context: a context,
// an interface of the app with its methods, or a type of the app that can become one. What a
// value of such a type carries goes into the context pool, and it carries what the pool holds.
// A context itself is a handle and carries nothing.
func (t *taint) contextual(typ types.Type) bool {
	c, ok := t.ctxTypes[typ]
	if !ok {
		if _, tuple := typ.(*types.Tuple); !tuple && t.ctx != nil {
			c = types.Implements(typ, t.ctx) || types.Implements(types.NewPointer(typ), t.ctx)
		}
		t.ctxTypes[typ] = c
	}
	return c
}

// shows returns what formatting a value of type typ reaches.
func (t *taint) shows(typ types.Type) *shown {
	s, ok := t.shown[typ]
	if !ok {
		s = &shown{}
		t.show(typ, s, false, map[showing]bool{})
		t.shown[typ] = s
	}
	return s
}

// show adds to s what formatting a value of type typ reaches, or with json, what encoding/json
// alone reaches. An unexported embedded pointer is walked for encoding/json alone, since fmt
// prints it as an address. It never looks into a context or another handle.
func (t *taint) show(typ types.Type, s *shown, json bool, seen map[showing]bool) {
	if seen[showing{typ, json}] {
		return
	}
	seen[showing{typ, json}] = true
	ms := t.prog.MethodSets.MethodSet(typ)
	names := implicitMethods
	if json {
		names = jsonMethods
	}
	for _, name := range names {
		if g := t.method(ms, name); g != nil && !slices.Contains(s.methods, g) {
			s.methods = append(s.methods, g)
		}
	}
	if g := t.method(ms, "Format"); !json && g != nil && isFormat(g.Signature) && !slices.Contains(s.formats, g) {
		s.formats = append(s.formats, g)
	}
	if t.contextual(typ) {
		s.context = s.context || !json
		return
	}
	if handles[named(typ)] {
		return
	}
	switch u := typ.Underlying().(type) {
	case *types.Pointer:
		t.show(u.Elem(), s, json, seen)
	case *types.Slice:
		t.show(u.Elem(), s, json, seen)
	case *types.Array:
		t.show(u.Elem(), s, json, seen)
	case *types.Map:
		t.show(u.Key(), s, json, seen)
		t.show(u.Elem(), s, json, seen)
	case *types.Struct:
		for f := range u.Fields() {
			_, ptr := f.Type().Underlying().(*types.Pointer)
			switch {
			case f.Exported():
				t.show(f.Type(), s, json, seen)
			case f.Embedded():
				t.show(f.Type(), s, json || ptr, seen)
			}
		}
	}
}

// method returns the method name of the method set ms when it has a body, or else nil.
func (t *taint) method(ms *types.MethodSet, name string) *ssa.Function {
	sel := ms.Lookup(nil, name)
	if sel == nil {
		return nil
	}
	if g := t.prog.FuncValue(sel.Obj().(*types.Func)); g != nil && g.Blocks != nil {
		return g
	}
	return nil
}

// isFormat reports whether sig is the signature of the Format method of fmt.Formatter, func(f
// fmt.State, verb rune).
func isFormat(sig *types.Signature) bool {
	p := sig.Params()
	return p.Len() == 2 && sig.Results().Len() == 0 && named(p.At(0).Type()) == "fmt.State" &&
		types.Identical(p.At(1).Type(), types.Universe.Lookup("rune").Type())
}

// checked returns where a recording call at pos is reported: at pos in a file of the app,
// generated or hand-written, and at the call that leads to it, token.NoPos, outside the app's
// files, such as in the wrapper of a method value.
func (t *taint) checked(pos token.Pos) token.Pos {
	if !pos.IsValid() || filepath.IsLocal(t.file(pos)) {
		return pos
	}
	return token.NoPos
}

// handWritten reports whether pos lies in a hand-written file of the app.
func (t *taint) handWritten(pos token.Pos) bool {
	if !pos.IsValid() {
		return false
	}
	name := t.file(pos)
	return filepath.IsLocal(name) && !t.a.generated[name]
}

// file returns the file that pos lies in, relative to the app folder when it lies in it.
func (t *taint) file(pos token.Pos) string {
	return t.a.rel(t.prog.Fset.PositionFor(pos, false).Filename)
}
