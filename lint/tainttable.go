package lint

import (
	"cmp"
	"go/token"
	"go/types"
	"maps"
	"slices"
)

// recorders are the functions and methods outside the app that record their arguments in a log,
// on standard output or in a span (E-LINT-008), by import path and name, a method as
// Type.Method. A method records every argument but its receiver.
var recorders = setOf(slices.Concat(
	prefixed("log/slog.", logCalls),
	prefixed("log/slog.Logger.", append(slices.Clone(logCalls), "WithGroup")),
	[]string{
		"log/slog.Handler.Handle", "log/slog.Handler.WithAttrs", "log/slog.Handler.WithGroup",
		"fmt.Print", "fmt.Printf", "fmt.Println",
		Framework + "/telemetry.Start", Framework + "/telemetry.Span.SetAttr",
	},
))

// logCalls are the functions of log/slog that log, and the methods of *slog.Logger with the same
// names.
var logCalls = []string{
	"Debug", "DebugContext", "Info", "InfoContext", "Warn", "WarnContext", "Error", "ErrorContext",
	"Log", "LogAttrs", "With",
}

// sources maps the functions and methods outside the app whose results are outside data to its
// kind. Their results also carry the arguments other than the receiver.
var sources = index(map[labels][]string{
	formValue: prefixed(Framework+"/web/form.", []string{
		"Input.GetValue", "Input.GetFormValue", "InputMultiple.GetValue", "InputMultiple.GetFormValue",
		"Select.GetValue", "SelectMultiple.GetValue", "Textarea.GetValue", "File.GetValue",
		"FileMultiple.GetValue",
	}),
	urlParam:  {Framework + "/web.Request.URLParam"},
	viewer:    {Framework + "/auth.Viewer"},
	dbValue:   {"database/sql.Rows.Scan", "database/sql.Row.Scan"},
	inboxMail: {Framework + "/mailer.List", Framework + "/mailer.Get"},
	storedFile: slices.Concat(
		prefixed(Framework+"/filestore.Store.", []string{
			"Stat", "ReadDir", "Open", "ReadFile", "Create", "WriteFile", "Mkdir", "Remove", "Rename",
		}),
		prefixed("io/fs.", []string{"ReadFile", "ReadDir", "Stat", "Glob", "Sub", "WalkDir"}),
	),
	appAnswer: {Framework + "/rpc.Call"},
	secret:    {Framework + "/secrets.Value.Reveal"},
})

// writers maps the functions and methods outside the app that write what their arguments carry
// into some of them, to those arguments, counted receiver first.
var writers = indexAll(map[int][]string{
	0: slices.Concat([]string{
		"bufio.Writer.ReadFrom", "bufio.Writer.Write", "bufio.Writer.WriteByte", "bufio.Writer.WriteRune",
		"bufio.Writer.WriteString",
		"bytes.Buffer.ReadFrom", "bytes.Buffer.Write", "bytes.Buffer.WriteByte", "bytes.Buffer.WriteRune",
		"bytes.Buffer.WriteString", "bytes.Reader.Reset",
		"container/heap.Push", "container/list.List.InsertAfter", "container/list.List.InsertBefore",
		"container/list.List.PushBack", "container/list.List.PushBackList", "container/list.List.PushFront",
		"container/list.List.PushFrontList",
		"encoding.BinaryUnmarshaler.UnmarshalBinary", "encoding.TextUnmarshaler.UnmarshalText",
		"encoding/binary.Append", "encoding/binary.AppendUvarint", "encoding/binary.AppendVarint", "encoding/binary.Encode",
		"encoding/binary.Write", "encoding/csv.Writer.Write", "encoding/csv.Writer.WriteAll",
		"encoding/hex.AppendDecode", "encoding/hex.AppendEncode", "encoding/hex.Decode", "encoding/hex.Encode",
		"encoding/json.Compact", "encoding/json.Encoder.Encode", "encoding/json.HTMLEscape", "encoding/json.Indent",
		"encoding/json.RawMessage.UnmarshalJSON",
		"encoding/xml.Encoder.Encode", "encoding/xml.Encoder.EncodeElement", "encoding/xml.Encoder.EncodeToken",
		"encoding/xml.Escape", "encoding/xml.EscapeText",
		"fmt.Append", "fmt.Appendf", "fmt.Appendln", "fmt.Fprint", "fmt.Fprintf", "fmt.Fprintln",
		"io.ByteWriter.WriteByte", "io.Copy", "io.CopyBuffer", "io.CopyN", "io.OffsetWriter.Write",
		"io.OffsetWriter.WriteAt", "io.PipeReader.CloseWithError", "io.PipeWriter.CloseWithError",
		"io.PipeWriter.Write", "io.ReaderFrom.ReadFrom", "io.StringWriter.WriteString", "io.WriteString",
		"io.Writer.Write", "io.WriterAt.WriteAt",
		"log/slog.Record.Add", "log/slog.Record.AddAttrs",
		"maps.Copy", "maps.Insert",
		"math/rand/v2.ChaCha8.UnmarshalBinary",
		"net/http.Header.Add", "net/http.Header.Set", "net/url.URL.UnmarshalBinary", "net/url.Values.Add",
		"net/url.Values.Set",
		"regexp.Regexp.UnmarshalText",
		"slices.AppendSeq", "slices.Insert", "slices.Replace",
		"strconv.AppendBool", "strconv.AppendFloat", "strconv.AppendInt", "strconv.AppendQuote", "strconv.AppendQuoteRune",
		"strconv.AppendQuoteRuneToASCII", "strconv.AppendQuoteRuneToGraphic", "strconv.AppendQuoteToASCII",
		"strconv.AppendQuoteToGraphic", "strconv.AppendUint",
		"strings.Builder.Write", "strings.Builder.WriteByte", "strings.Builder.WriteRune",
		"strings.Builder.WriteString", "strings.Reader.Reset",
		"sync.Map.CompareAndSwap", "sync.Map.LoadOrStore", "sync.Map.Store", "sync.Map.Swap", "sync.Pool.Put",
		"sync/atomic.Pointer.CompareAndSwap", "sync/atomic.Pointer.Store", "sync/atomic.Pointer.Swap",
		"sync/atomic.Value.CompareAndSwap", "sync/atomic.Value.Store", "sync/atomic.Value.Swap",
		"unicode/utf8.AppendRune", "unicode/utf8.EncodeRune",
	}, prefixed(Framework+"/", []string{
		"telemetry.Span.SetAttr",
		"web.Frame.SetAssets",
		"web/form.BaseFormValues.SetElements", "web/form.BaseFormValues.SetError",
		"web/form.File.SetError", "web/form.FileMultiple.SetError",
		"web/form.Input.SetError", "web/form.Input.SetValue", "web/form.InputMultiple.SetError",
		"web/form.InputMultiple.SetValue",
		"web/form.Select.SetError", "web/form.Select.SetOptions", "web/form.Select.SetValue",
		"web/form.SelectMultiple.SetError", "web/form.SelectMultiple.SetOptions", "web/form.SelectMultiple.SetValue",
		"web/form.Textarea.SetError", "web/form.Textarea.SetValue",
	})),
	1: {
		"bufio.Reader.Read", "bufio.Reader.WriteTo",
		"bytes.Buffer.Read", "bytes.Buffer.WriteTo", "bytes.Reader.Read", "bytes.Reader.ReadAt",
		"bytes.Reader.WriteTo",
		"database/sql.Rows.Scan", "database/sql.Row.Scan",
		"encoding.BinaryAppender.AppendBinary", "encoding.TextAppender.AppendText",
		"encoding/base32.Encoding.AppendDecode", "encoding/base32.Encoding.AppendEncode", "encoding/base32.Encoding.Decode",
		"encoding/base32.Encoding.Encode",
		"encoding/base64.Encoding.AppendDecode", "encoding/base64.Encoding.AppendEncode", "encoding/base64.Encoding.Decode",
		"encoding/base64.Encoding.Encode",
		"encoding/json.Decoder.Decode", "encoding/json.Unmarshal",
		"encoding/xml.Decoder.Decode", "encoding/xml.Decoder.DecodeElement", "encoding/xml.Unmarshal",
		"errors.As",
		"fmt.Fscan", "fmt.Fscanln", "fmt.Sscan", "fmt.Sscanln",
		"hash.Hash.Sum",
		"io.LimitedReader.Read", "io.PipeReader.Read", "io.ReadAtLeast", "io.ReadFull", "io.Reader.Read",
		"io.ReaderAt.ReadAt", "io.SectionReader.Read", "io.SectionReader.ReadAt", "io.WriterTo.WriteTo",
		"io/fs.File.Read",
		"math/rand/v2.ChaCha8.AppendBinary", "math/rand/v2.ChaCha8.Read",
		"net/http.Header.Write", "net/url.URL.AppendBinary",
		"regexp.Regexp.AppendText", "regexp.Regexp.Expand", "regexp.Regexp.ExpandString",
		"strings.Reader.Read", "strings.Reader.ReadAt", "strings.Reader.WriteTo", "strings.Replacer.WriteString",
		"time.Time.AppendFormat",
		Framework + "/web/form.SelectOption.WriteHtml", Framework + "/web/form.SelectOptionElement.WriteHtml",
		Framework + "/web/form.SelectOptionGroup.WriteHtml",
	},
	2: {"encoding/binary.Decode", "encoding/binary.Read", "fmt.Fscanf", "fmt.Sscanf", "io.CopyBuffer"},
})

// keepers maps the functions outside the app whose result keeps one of their arguments and writes
// into it, such as an encoder of a writer, to that argument.
var keepers = index(map[int][]string{
	0: {
		"bufio.NewWriter", "bufio.NewWriterSize", "bytes.NewBuffer", "encoding/csv.NewWriter",
		"encoding/hex.Dumper", "encoding/hex.NewEncoder", "encoding/json.NewEncoder", "encoding/xml.NewEncoder",
		"io.MultiWriter", "io.NewOffsetWriter", "log/slog.NewJSONHandler", "log/slog.NewTextHandler",
	},
	1: {"bufio.NewReadWriter", "encoding/base32.NewEncoder", "encoding/base64.NewEncoder", "io.TeeReader"},
})

// links maps the functions and methods outside the app after whose call two of its operands
// refer to the same memory, to the two: the receiver and an argument it keeps, or two results,
// such as the ends of a pipe. An operand is an argument, counted receiver first, or a result,
// counted after the arguments.
var links = map[string][2]int{
	"bufio.Reader.Reset":   {0, 1},
	"bufio.Scanner.Buffer": {0, 1},
	"bufio.Writer.Reset":   {0, 1},
	"io.Pipe":              {0, 1},
}

// index returns the map from each name of groups to the key of its group.
func index[K comparable](groups map[K][]string) map[string]K {
	m := map[string]K{}
	for k, names := range groups {
		for _, n := range names {
			m[n] = k
		}
	}
	return m
}

// indexAll returns the map from each name of groups to the keys of the groups it is in, in
// order.
func indexAll[K cmp.Ordered](groups map[K][]string) map[string][]K {
	m := map[string][]K{}
	for k, names := range groups {
		for _, n := range names {
			m[n] = append(m[n], k)
		}
	}
	for _, ks := range m {
		slices.Sort(ks)
	}
	return m
}

// A pool holds what values of one sort pass between parts of the app without a variable of
// its own, such as the values a context carries.
type pool string

const (
	contextPool  pool = "context"
	panicPool    pool = "panic"
	reactivePool pool = "reactive"
	// statePool holds what the receivers of the hooks keep between requests: the data
	// providers, and what they refer to.
	statePool pool = "state"
)

// putters maps the functions and methods outside the app that put arguments into a pool to the
// pool and those arguments, counted receiver first. A call of a function value func(error), such
// as the cancel of context.WithCancelCause, puts its argument into the context pool too.
var putters = map[string]struct {
	pool pool
	args []int
}{
	"context.WithDeadlineCause":                   {contextPool, []int{2}},
	"context.WithTimeoutCause":                    {contextPool, []int{2}},
	"context.WithValue":                           {contextPool, []int{1, 2}},
	Framework + "/web/reactive.Topic.Publish":     {reactivePool, []int{2}},
	Framework + "/web/reactive.Broadcast.Publish": {reactivePool, []int{1}},
}

// causeFunc is func(error), the signature of a function that cancels a context with a cause,
// whatever its type.
var causeFunc = types.NewSignatureType(nil, nil, nil,
	types.NewTuple(types.NewParam(token.NoPos, nil, "", types.Universe.Lookup("error").Type())), nil, false)

// getters maps the functions and methods outside the app whose results come from a pool to the
// pool.
var getters = map[string]pool{
	"context.Cause":                                  contextPool,
	"context.Context.Value":                          contextPool,
	Framework + "/web/reactive.TopicSub.Updates":     reactivePool,
	Framework + "/web/reactive.BroadcastSub.Updates": reactivePool,
}

// handles are the types whose values never carry outside data themselves: what is read through
// them is a source or comes from a pool.
var handles = setOf([]string{
	"context.Context",
	"database/sql.Conn", "database/sql.DB", "database/sql.Stmt", "database/sql.Tx",
	"time.Location", "time.Time",
	Framework + "/filestore.Store",
	Framework + "/telemetry.Span",
	Framework + "/web/reactive.Broadcast", Framework + "/web/reactive.BroadcastSub",
	Framework + "/web/reactive.Topic", Framework + "/web/reactive.TopicSub",
})

// requests are the types whose values are the request a page serves.
var requests = setOf([]string{Framework + "/web.Request", "net/http.Request"})

// tabled holds every function and method that a table names.
var tabled = setOf(slices.Concat(
	slices.Collect(maps.Keys(recorders)), slices.Collect(maps.Keys(sources)), slices.Collect(maps.Keys(writers)),
	slices.Collect(maps.Keys(keepers)), slices.Collect(maps.Keys(links)), slices.Collect(maps.Keys(putters)),
	slices.Collect(maps.Keys(getters)),
))

// tableMethods returns every method that the tables name.
func tableMethods() []string {
	var out []string
	for n := range tabled {
		if isMethod(n) {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return out
}

// named returns the import path and name of t's type name, as the tables write it, or "" when t
// is not a named type.
func named(t types.Type) string {
	n, ok := types.Unalias(t).(*types.Named)
	if !ok || n.Obj().Pkg() == nil {
		return ""
	}
	return n.Obj().Pkg().Path() + "." + n.Obj().Name()
}

// isText reports whether a value of type t is text, an error or another value that can hold
// text without holding a value of another type: a string, a byte, a rune, a slice or an array of
// bytes or runes, an interface, a function or a type parameter.
func isText(t types.Type) bool {
	if b, ok := types.Unalias(t).(*types.Basic); ok {
		return b.Info()&types.IsString != 0 || b == types.Universe.Lookup("byte").Type() || b == types.Universe.Lookup("rune").Type()
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		return u.Info()&types.IsString != 0
	case *types.Slice:
		return isChar(u.Elem())
	case *types.Array:
		return isChar(u.Elem())
	case *types.Interface, *types.Signature, *types.TypeParam:
		return true
	}
	return false
}

// isChar reports whether t is an integer type of the size of a byte or a rune.
func isChar(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && (b.Kind() == types.Uint8 || b.Kind() == types.Int32)
}

// isRef reports whether a value of type t refers to memory that other values can share.
func isRef(t types.Type) bool {
	switch t.Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Signature, *types.Interface, *types.TypeParam:
		return true
	}
	return false
}

// holds reports whether t, or a type that a value of t holds, is one that is reports true for.
// It never looks into a handle.
func holds(t types.Type, is func(types.Type) bool, seen map[types.Type]bool) bool {
	if handles[named(t)] || seen[t] {
		return false
	}
	if is(t) {
		return true
	}
	seen[t] = true
	switch u := t.Underlying().(type) {
	case *types.Pointer:
		return holds(u.Elem(), is, seen)
	case *types.Slice:
		return holds(u.Elem(), is, seen)
	case *types.Array:
		return holds(u.Elem(), is, seen)
	case *types.Chan:
		return holds(u.Elem(), is, seen)
	case *types.Map:
		return holds(u.Key(), is, seen) || holds(u.Elem(), is, seen)
	case *types.Struct:
		for f := range u.Fields() {
			if holds(f.Type(), is, seen) {
				return true
			}
		}
	case *types.Tuple:
		for v := range u.Variables() {
			if holds(v.Type(), is, seen) {
				return true
			}
		}
	}
	return false
}

// isContext reports whether a value of type t is a context: an interface among the handles, so
// context.Context. What comes out of one comes from the context pool.
func isContext(t types.Type) bool {
	return types.IsInterface(t) && handles[named(t)]
}

// isRequest reports whether a value of type t is the request a page serves, or points to it.
func isRequest(t types.Type) bool {
	if p, ok := t.Underlying().(*types.Pointer); ok {
		t = p.Elem()
	}
	return requests[named(t)]
}
