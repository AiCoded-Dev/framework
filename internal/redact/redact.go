// Package redact describes errors and panic values by their kinds and codes, never their text,
// for the logs and spans the framework records anywhere but under aicoded dev in environment
// `dev`.
package redact

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"slices"
	"strings"

	"connectrpc.com/connect"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/runner"
)

// Dev reports whether ctx carries a runner session of aicoded dev in environment `dev`, whose
// data is synthetic.
func Dev(ctx context.Context) bool {
	s := runner.From(ctx)
	return s != nil && s.Env == "dev"
}

// For returns err's text under aicoded dev in environment `dev`, and Error(err) in any other
// environment or with no runner.
func For(ctx context.Context, err error) string {
	if Dev(ctx) && err != nil {
		return err.Error()
	}
	return Error(err)
}

// ValueFor returns a panic's value as fmt.Sprint writes it under aicoded dev in environment
// `dev`, and Value(v) in any other environment or with no runner.
func ValueFor(ctx context.Context, v any) string {
	if Dev(ctx) {
		return fmt.Sprint(v)
	}
	return Value(v)
}

// Value describes a panic's value: Error for an error, and the value's Go type otherwise.
func Value(v any) string {
	if err, ok := v.(error); ok {
		return Error(err)
	}
	return fmt.Sprintf("%T", v)
}

// Error describes err without its text: every error of err's tree, outer first, by its kind.
// A kind is the name of a sentinel, an E-code, a status code or another kind that Kind added, and
// otherwise the error's Go type. A wrapped error follows its wrapper after ": ", and the errors
// that one error joins follow it in brackets. The errors of fmt.Errorf and errors.Join have no
// kind of their own, and the error a *connect.Error wraps is its message, so it is left out.
// A nil err is "<nil>", as fmt writes it.
func Error(err error) string {
	if err == nil {
		return "<nil>"
	}
	var b strings.Builder
	write(&b, err)
	return b.String()
}

type sentinel struct {
	err  error
	name string
}

var (
	kinds     = map[reflect.Type]func(error) string{}
	sentinels []sentinel
	joins     = map[reflect.Type]bool{
		reflect.TypeOf(fmt.Errorf("%w", context.Canceled)):                      true,
		reflect.TypeOf(fmt.Errorf("%w %w", context.Canceled, context.Canceled)): true,
		reflect.TypeOf(errors.Join(context.Canceled)):                           true,
	}
	leaves = map[reflect.Type]bool{reflect.TypeFor[*connect.Error](): true}
)

// Kind makes Error name every error of type T by kind. Packages call it from init.
func Kind[T error](kind func(T) string) {
	kinds[reflect.TypeFor[T]()] = func(err error) string {
		var t T
		if errors.As(err, &t) {
			return kind(t)
		}
		return ""
	}
}

// Sentinel makes Error name err, a sentinel error, as name. Packages call it from init.
func Sentinel(err error, name string) {
	sentinels = append(sentinels, sentinel{err, name})
}

// fsOps are the operations of an *fs.PathError that its kind names: those of file stores and
// of io/fs.
var fsOps = map[string]bool{
	"create": true, "mkdir": true, "open": true, "read": true, "readdir": true, "remove": true,
	"rename": true, "stat": true, "sub": true, "write": true,
}

func init() {
	Sentinel(context.Canceled, "context.Canceled")
	Sentinel(context.DeadlineExceeded, "context.DeadlineExceeded")
	Sentinel(fs.ErrInvalid, "fs.ErrInvalid")
	Sentinel(fs.ErrPermission, "fs.ErrPermission")
	Sentinel(fs.ErrExist, "fs.ErrExist")
	Sentinel(fs.ErrNotExist, "fs.ErrNotExist")
	Sentinel(fs.ErrClosed, "fs.ErrClosed")
	Kind(func(e *errs.Error) string { return e.Code })
	Kind(func(e *fs.PathError) string {
		if fsOps[e.Op] {
			return "fs.PathError " + e.Op
		}
		return "fs.PathError"
	})
	Kind(func(e *connect.Error) string {
		if coded, ok := errs.Parse(e.Message()); ok {
			return e.Code().String() + " " + coded.Code
		}
		return e.Code().String()
	})
}

func write(b *strings.Builder, err error) {
	t := reflect.TypeOf(err)
	var next []error
	if !leaves[t] {
		if e := errors.Unwrap(err); e != nil {
			next = []error{e}
		} else if u, ok := err.(interface{ Unwrap() []error }); ok {
			next = slices.DeleteFunc(slices.Clone(u.Unwrap()), func(e error) bool { return e == nil })
		}
	}
	kind := kindOf(err, t, len(next) == 0)
	b.WriteString(kind)
	if len(next) == 0 {
		return
	}
	if kind != "" {
		b.WriteString(": ")
	}
	if len(next) == 1 {
		write(b, next[0])
		return
	}
	b.WriteString("[")
	for i, e := range next {
		if i > 0 {
			b.WriteString(", ")
		}
		write(b, e)
	}
	b.WriteString("]")
}

// kindOf returns the kind of err itself, whose type is t, or "" for an error that only joins
// others. alone reports whether err wraps no error.
func kindOf(err error, t reflect.Type, alone bool) string {
	if alone {
		for _, s := range sentinels {
			if errors.Is(err, s.err) {
				return s.name
			}
		}
	}
	if kind, ok := kinds[t]; ok {
		if k := kind(err); k != "" {
			return k
		}
	}
	if joins[t] && !alone {
		return ""
	}
	return t.String()
}
