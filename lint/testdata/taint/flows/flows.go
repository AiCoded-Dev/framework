// Package flows carries a URL parameter through each kind of value and call before it records
// it.
package flows

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"sort"
	"strings"

	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/web"
	"aicoded.dev/framework/web/reactive"
)

// user is a person.
type user struct {
	Login string
	Team  string
	ID    int
}

// loginKey is the key of a login in a context.
type loginKey struct{}

// Values carries the URL parameter login through values.
func Values(ctx context.Context, r *web.Request) {
	login := r.URLParam("login")
	slog.Info("sprintf", "v", fmt.Sprintf("user %s", login)) // leak: a URL parameter reaches slog.Info
	slog.Info("concat", "v", "user "+login)                  // leak: a URL parameter reaches slog.Info
	slog.Info("bytes", "v", []byte(login))                   // leak: a URL parameter reaches slog.Info
	slog.Info("rune", "v", string([]rune(login)[0]))         // leak: a URL parameter reaches slog.Info
	u := user{Login: login, Team: "ops", ID: 7}
	slog.Info("field", "team", u.Team) // leak: a URL parameter reaches slog.Info
	slog.Info("field", "id", u.ID)     // clean
	names := append([]string{"ops"}, login)
	slog.Info("slice", "first", names[0]) // leak: a URL parameter reaches slog.Info
	seen := map[string]bool{login: true}
	for k := range seen {
		slog.Info("map key", "k", k) // leak: a URL parameter reaches slog.Info
	}
	byID := map[int]string{1: login}
	slog.Info("map value", "v", byID[1]) // leak: a URL parameter reaches slog.Info
	ch := make(chan string, 1)
	ch <- login
	slog.Info("channel", "v", <-ch) // leak: a URL parameter reaches slog.Info
	ctx = context.WithValue(ctx, loginKey{}, login)
	slog.InfoContext(ctx, "context", "v", ctx.Value(loginKey{})) // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.InfoContext
	sum := sha256.Sum256([]byte(login))
	slog.Info("hash", "v", hex.EncodeToString(sum[:])) // leak: a URL parameter reaches slog.Info
	err := errors.New("no user " + login)
	slog.Error("errors.New", "err", err) // leak: a URL parameter reaches slog.Error
	wrapped := fmt.Errorf("lookup: %w", err)
	slog.Error("wrapped", "err", wrapped)                     // leak: a URL parameter reaches slog.Error
	slog.Error("joined", "err", errors.Join(io.EOF, wrapped)) // leak: a URL parameter reaches slog.Error
	var b strings.Builder
	b.WriteString(login)
	slog.Info("builder", "v", b.String()) // leak: a URL parameter reaches slog.Info
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(u)
	slog.Info("encoder", "v", buf.String()) // leak: a URL parameter reaches slog.Info
	j, _ := json.Marshal(u)
	var back user
	_ = json.Unmarshal(j, &back)
	slog.Info("unmarshal", "team", back.Team) // leak: a URL parameter reaches slog.Info
	for k := range maps.Keys(seen) {
		slog.Info("iterator", "k", k) // leak: a URL parameter reaches slog.Info
	}
	slog.InfoContext(ctx, "two kinds", "v", login+auth.Viewer(ctx).Name) // leak: a URL parameter or the viewer's identity reaches slog.InfoContext
}

// Calls carries the URL parameter login through calls.
func Calls(r *web.Request) {
	login := r.URLParam("login")
	greet := func() {
		slog.Info("closure", "login", login) // leak: a URL parameter reaches slog.Info
	}
	greet()
	defer func() {
		fmt.Println(login) // leak: a URL parameter reaches fmt.Println
	}()
	go func(s string) {
		println(s) // leak: a URL parameter reaches println
	}(login)
	upper := func(s string) string { return strings.ToUpper(s) }
	slog.Info("closure result", "v", upper(login)) // leak: a URL parameter reaches slog.Info
	names := []string{login, "ops"}
	sort.Slice(names, func(i, j int) bool {
		slog.Info("sort", "a", names[i]) // leak: a URL parameter reaches slog.Info
		return names[i] < names[j]
	})
	outer(login)
	slog.Info("two calls deep", "v", wrap(login)) // leak: a URL parameter reaches slog.Info
	var n namer = person{name: login}
	slog.Info("interface", "name", n.Name()) // leak: a URL parameter reaches slog.Info
	var rep reporter = logReporter{}
	rep.Report(login)
	remember(login)
	showLast()
	tags[1] = login
	slog.Info("stringer", "tag", tag(1)) // leak: a URL parameter reaches slog.Info
	mustLogin(login)
}

// outer passes s on, two calls deep.
func outer(s string) { middle(s) }

// middle passes s on.
func middle(s string) { inner(s) }

// inner records s.
func inner(s string) {
	slog.Info("two calls deep", "s", s) // leak: a URL parameter reaches slog.Info
}

// wrap returns s in brackets, two calls deep.
func wrap(s string) string { return bracket(s) }

// bracket returns s in brackets.
func bracket(s string) string { return "[" + s + "]" }

// namer has a name.
type namer interface{ Name() string }

// person is a namer.
type person struct{ name string }

// Name returns the name of p.
func (p person) Name() string { return p.name }

// reporter reports a value.
type reporter interface{ Report(s string) }

// logReporter reports to the log.
type logReporter struct{}

// Report records s.
func (logReporter) Report(s string) {
	slog.Info("report", "s", s) // leak: a URL parameter reaches slog.Info
}

// last is the last login remembered.
var last string

// remember keeps s in a global variable.
func remember(s string) { last = s }

// showLast records the last login remembered.
func showLast() {
	slog.Info("global", "v", last) // leak: a URL parameter reaches slog.Info
}

// tag is a number that its String method turns into a login.
type tag int

// tags holds the login of each tag.
var tags = map[tag]string{}

// String returns the login of t.
func (t tag) String() string { return tags[t] }

// mustLogin panics with login, and records what it recovers.
func mustLogin(login string) {
	defer func() {
		if v := recover(); v != nil {
			slog.Error("panic", "v", v) // leak: a URL parameter reaches slog.Error
		}
	}()
	panic("no user " + login)
}

// Live carries the URL parameter login through a topic of live values.
func Live(r *web.Request, topic *reactive.Topic[string, string]) {
	sub := topic.Subscribe("k")
	defer sub.Close()
	topic.Publish("k", r.URLParam("login"))
	slog.Info("topic", "v", <-sub.Updates()) // leak: a URL parameter reaches slog.Info
}

// Visits holds the logins that opened a page.
type Visits struct{ logins []string }

// KeepDP provides a page that keeps the login of its URL.
type KeepDP struct{ v *Visits }

// Data keeps the URL parameter login.
func (p *KeepDP) Data(_ context.Context, r *web.Request) error {
	p.v.logins = append(p.v.logins, r.URLParam("login"))
	return nil
}

// ShowDP provides another page, which records what the first one kept.
type ShowDP struct{ v *Visits }

// Data records the logins kept.
func (p *ShowDP) Data(ctx context.Context, _ *web.Request) error {
	slog.InfoContext(ctx, "visits", "n", len(p.v.logins)) // clean
	slog.InfoContext(ctx, "visits", "logins", p.v.logins) // leak: a URL parameter reaches slog.InfoContext
	return nil
}

// Causes carries the URL parameter login through the cause of a context.
func Causes(ctx context.Context, r *web.Request) {
	login := r.URLParam("login")
	cctx, cancel := context.WithCancelCause(ctx)
	cancel(errors.New("no user " + login))
	slog.Error("cause", "err", context.Cause(cctx)) // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.Error
	tctx, stop := context.WithTimeoutCause(ctx, 0, errors.New("no user "+login))
	defer stop()
	slog.Error("timeout cause", "err", context.Cause(tctx)) // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.Error
}

// Buffers carries the URL parameter login through a buffer over the app's own bytes.
func Buffers(r *web.Request) {
	login := r.URLParam("login")
	mem := make([]byte, 0, 64)
	_, _ = bytes.NewBuffer(mem).WriteString(login)
	slog.Info("buffer", "v", mem[:cap(mem)]) // leak: a URL parameter reaches slog.Info
}

// viewerCtx is a context of the app's own type that holds the viewer's name, read by a method
// with a value receiver.
type viewerCtx struct {
	context.Context
	name string
}

// Value returns the name, whatever the key.
func (c viewerCtx) Value(any) any { return c.name }

// viewerPtrCtx is a context of the app's own type that holds the viewer's name, read by methods
// with a pointer receiver.
type viewerPtrCtx struct {
	context.Context
	name string
}

// Value returns the name, whatever the key.
func (c *viewerPtrCtx) Value(any) any { return c.name }

// Err returns an error that holds the name.
func (c *viewerPtrCtx) Err() error { return errors.New("no access for " + c.name) }

// Contexts carries the viewer's name through contexts of the app's own types. Every context of
// the app shares what they hold, so each line that reads one, here, in Values, in Causes and in
// contexts.go, names every kind that any of them holds.
func Contexts(ctx context.Context) {
	name := auth.Viewer(ctx).Name
	vctx := context.Context(viewerCtx{ctx, name})
	slog.InfoContext(vctx, "seen")                    // clean
	slog.Info("value receiver", "v", vctx.Value(nil)) // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.Info
	var pctx context.Context = &viewerPtrCtx{ctx, name}
	slog.Info("pointer receiver", "v", pctx.Value(nil))  // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.Info
	slog.Info("asserted", "name", vctx.(viewerCtx).name) // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.Info
	slog.Error("method", "err", pctx.Err())              // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.Error
	slog.Info("context", "ctx", pctx)                    // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.Info
}

// Builtins carries the URL parameter login through copy and select.
func Builtins(r *web.Request) {
	login := r.URLParam("login")
	b := make([]byte, len(login))
	copy(b, login)
	slog.Info("copy", "v", b) // leak: a URL parameter reaches slog.Info
	ch := make(chan string, 1)
	select {
	case ch <- login:
	default:
	}
	select {
	case v := <-ch:
		slog.Info("select", "v", v) // leak: a URL parameter reaches slog.Info
	default:
	}
}

// box holds a tag in an exported field.
type box struct{ T tag }

// hidden holds a tag in an unexported field.
type hidden struct{ t tag }

// Held carries the URL parameter login through the String method of a tag that another value
// holds. fmt and log/slog call it on an exported field, an element or what a pointer points to,
// and on no unexported field.
func Held(r *web.Request) {
	tags[2] = r.URLParam("login")
	slog.Info("struct", "v", box{T: 2})           // leak: a URL parameter reaches slog.Info
	slog.Info("pointer", "v", &box{T: 2})         // leak: a URL parameter reaches slog.Info
	slog.Info("slice", "v", []tag{2})             // leak: a URL parameter reaches slog.Info
	slog.Info("map", "v", map[string]tag{"a": 2}) // leak: a URL parameter reaches slog.Info
	fmt.Printf("%v\n", box{T: 2})                 // leak: a URL parameter reaches fmt.Printf
	j, _ := json.Marshal(box{T: 2})
	slog.Info("json", "v", string(j))          // leak: a URL parameter reaches slog.Info
	slog.Info("unexported", "v", hidden{t: 2}) // clean
}

// same returns b.
func same(b []byte) []byte { return b }

// Aliases carries the URL parameter login into the app's own bytes through a slice that a
// function of the app returns and that shares their memory.
func Aliases(r *web.Request) {
	mem := make([]byte, 0, 64)
	alias := same(mem)
	_ = append(alias, r.URLParam("login")...)
	slog.Info("alias", "v", mem[:cap(mem)]) // leak: a URL parameter reaches slog.Info
}

// tagged holds a tag in an exported field.
type tagged struct{ T tag }

// embeds embeds a tagged, whose exported fields fmt and log/slog print with their methods.
type embeds struct{ tagged }

// embedsPtr embeds a pointer to a tagged, which fmt prints as an address.
type embedsPtr struct{ *tagged }

// keepsTagged holds a tagged in an unexported field that is not embedded.
type keepsTagged struct{ in tagged }

// note is a number that encoding/json writes as the text its MarshalJSON method looks up.
type note int

// MarshalJSON returns the text of n as JSON.
func (n note) MarshalJSON() ([]byte, error) { return json.Marshal(tags[tag(n)]) }

// noted holds a note in an exported field.
type noted struct{ N note }

// notedPtr embeds a pointer to a noted, whose exported fields encoding/json writes.
type notedPtr struct{ *noted }

// ftag is a number that fmt writes as the text its Format method looks up.
type ftag int

// Format writes the text of t.
func (t ftag) Format(f fmt.State, _ rune) { _, _ = fmt.Fprint(f, tags[tag(t)]) }

// HeldEmbedded carries the URL parameter login through the methods of values that embedded
// fields hold, and through Format.
func HeldEmbedded(r *web.Request) {
	tags[3] = r.URLParam("login")
	slog.Info("embedded", "v", embeds{tagged{T: 3}})             // leak: a URL parameter reaches slog.Info
	fmt.Printf("%v\n", embeds{tagged{T: 3}})                     // leak: a URL parameter reaches fmt.Printf
	slog.Info("embedded pointer", "v", embedsPtr{&tagged{T: 3}}) // clean
	slog.Info("json", "v", notedPtr{&noted{N: 3}})               // leak: a URL parameter reaches slog.Info
	slog.Info("unexported", "v", keepsTagged{in: tagged{T: 3}})  // clean
	slog.Info("format", "v", ftag(3))                            // leak: a URL parameter reaches slog.Info
	fmt.Printf("%v\n", ftag(3))                                  // leak: a URL parameter reaches fmt.Printf
	slog.Info("held format", "v", []ftag{3})                     // leak: a URL parameter reaches slog.Info
}
