package web

import (
	"context"
	"fmt"
	"io"
	"reflect"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/identity"
)

// Access is a route's <ssr:access> rule: the viewer needs one of Roles. The role "*" admits
// every viewer the company login lets into the app.
type Access struct {
	Roles []string
}

func (a Access) allows(v identity.Identity) bool {
	for _, role := range a.Roles {
		if role == "*" || v.HasRole(role) {
			return true
		}
	}
	return false
}

// Route is the code aicoded generate writes for one folder under pages/. Apps do not implement it.
type Route interface {
	// Access returns the route's own <ssr:access> rule; the zero Access means it has none.
	Access() Access
	// Layout reports whether the route's template has <ssr:content/>, so that the routes below
	// it render inside it. A route with routes below it that is not a layout is a gate: its
	// access rule and Guard apply to every route below it, and it renders only as its own page.
	Layout() bool
	// NewState returns the route's part of one request.
	NewState() RouteState
}

// RouteState is one route's part of a request. The framework calls Guard for every route on
// the path, root first, and then InitForms, SubmitForm, Data and Write for the routes that
// render the page: the layouts on the path and the page itself. Generated states embed Frame.
type RouteState interface {
	Guard(ctx context.Context, r *Request) error
	InitForms(ctx context.Context, r *Request, w ResponseWriter) error
	// SubmitForm validates and processes the posted form id if this route renders it, and
	// reports whether it does.
	SubmitForm(ctx context.Context, r *Request, w ResponseWriter, id string) (bool, error)
	Data(ctx context.Context, r *Request, w ResponseWriter) error
	DefaultRoute(ctx context.Context, r *Request) (string, error)
	Write(w io.Writer) error
	frame() *Frame
}

// CheckNoGuard panics with E-WEB-007 when dp, the data provider of the route at path, has a
// method named Guard, of any signature, its own or one from an embedded type, while the
// route's <ssr:access> has no guard="true", so the framework would never call it. Generated
// NewRoute calls it; apps do not.
//
// Generated code only.
func CheckNoGuard(dp any, path string) {
	t := reflect.TypeOf(dp)
	if t == nil {
		return
	}
	_, own := t.MethodByName("Guard")
	_, onPointer := reflect.PointerTo(t).MethodByName("Guard")
	if own || onPointer {
		panic(errs.New("E-WEB-007",
			fmt.Sprintf(`%T of %s has a Guard method, but the route has no <ssr:access> with guard="true", so Guard is never called`, dp, path),
			`add guard="true" to this route's <ssr:access>, or remove the Guard method`))
	}
}

// Frame joins a route's state to the route rendered inside it. Generated states embed it.
type Frame struct {
	child  RouteState
	assets []string
}

func (f *Frame) frame() *Frame { return f }

// SetAssets sets the tags <ssr:assets/> writes for this route.
func (f *Frame) SetAssets(tags []string) { f.assets = tags }

// WriteChild writes the route below this one, where <ssr:content/> stands.
func (f *Frame) WriteChild(w io.Writer) error {
	if f.child == nil {
		return nil
	}
	return f.child.Write(w)
}

// WriteAssets writes the asset tags of this route and of every route below it, each once.
func (f *Frame) WriteAssets(w io.Writer) error {
	seen := map[string]bool{}
	for fr := f; fr != nil; {
		for _, tag := range fr.assets {
			if seen[tag] {
				continue
			}
			seen[tag] = true
			if _, err := io.WriteString(w, tag); err != nil {
				return err
			}
		}
		if fr.child == nil {
			break
		}
		fr = fr.child.frame()
	}
	return nil
}
