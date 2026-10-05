package filestore

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
	"aicoded.dev/framework/runnerproto/socket"
)

// fake is an in-memory FilesService with the one store "docs". It answers with the status codes
// of files.proto, streams reads in chunks of 7 bytes and commits a write only when its stream ends.
// With reversed set, ReadDir lists entries in reverse order, which files.proto allows.
type fake struct {
	calls    atomic.Int64
	reversed atomic.Bool
	mu       sync.Mutex
	files    map[string][]byte
	dirs     map[string]bool
	spans    []*runnerv1.Span // exported by the test's goroutine
}

// fakeRunner returns a context whose session talks to a fake over a Unix socket, and a function
// reporting how many calls the fake has served.
func fakeRunner(t *testing.T) (context.Context, func() int) {
	ctx, f := newFake(t)
	return ctx, func() int { return int(f.calls.Load()) }
}

func newFake(t *testing.T) (context.Context, *fake) {
	f := &fake{files: map[string][]byte{}, dirs: map[string]bool{".": true}}
	dir, err := os.MkdirTemp("", "aicoded-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, runnerproto.RunnerSocket)
	l, err := socket.Listen(t.Context(), sock)
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.Handle(runnerv1connect.NewFilesServiceHandler(f))
	srv := socket.NewServer(mux)
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	client := runnerv1connect.NewFilesServiceClient(socket.Client(sock), socket.BaseURL, connect.WithGRPC())
	export := func(s *runnerv1.Span) { f.spans = append(f.spans, s) }
	return runner.With(context.Background(), &runner.Session{Files: client, Export: export}), f
}

func status(code connect.Code, name string) error {
	return connect.NewError(code, fmt.Errorf("%q", name))
}

// do counts the call, checks the store and the names, and runs fn under the lock. A change may
// not target the store itself, and no name part may start with the runner's temporary prefix.
func (f *fake) do(store string, change bool, names []string, fn func() error) error {
	f.calls.Add(1)
	if store != "docs" {
		return connect.NewError(connect.CodePermissionDenied, errors.New("the file store is not declared"))
	}
	for _, n := range names {
		if !fs.ValidPath(n) || change && n == "." || strings.Contains("/"+n, "/.aicoded-tmp-") {
			return status(connect.CodeInvalidArgument, n)
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return fn()
}

// stat walks the folders above name from the top, as the operating system does.
func (f *fake) stat(name string) (*runnerv1.FileInfo, error) {
	elems := strings.Split(name, "/")
	for i := 1; i < len(elems); i++ {
		p := strings.Join(elems[:i], "/")
		if _, ok := f.files[p]; ok {
			return nil, status(connect.CodeFailedPrecondition, name)
		}
		if !f.dirs[p] {
			return nil, status(connect.CodeNotFound, name)
		}
	}
	if data, ok := f.files[name]; ok {
		return &runnerv1.FileInfo{Name: path.Base(name), Size: int64(len(data))}, nil
	}
	if f.dirs[name] {
		return &runnerv1.FileInfo{Name: path.Base(name), Dir: true}, nil
	}
	return nil, status(connect.CodeNotFound, name)
}

// list returns the folder's entries sorted by name.
func (f *fake) list(name string) []*runnerv1.FileInfo {
	var out []*runnerv1.FileInfo
	for k := range f.dirs {
		if k != "." && path.Dir(k) == name {
			out = append(out, &runnerv1.FileInfo{Name: path.Base(k), Dir: true})
		}
	}
	for k, v := range f.files {
		if path.Dir(k) == name {
			out = append(out, &runnerv1.FileInfo{Name: path.Base(k), Size: int64(len(v))})
		}
	}
	slices.SortFunc(out, func(a, b *runnerv1.FileInfo) int { return strings.Compare(a.GetName(), b.GetName()) })
	return out
}

func (f *fake) Stat(_ context.Context, r *connect.Request[runnerv1.StatRequest]) (*connect.Response[runnerv1.StatResponse], error) {
	resp := &runnerv1.StatResponse{}
	return connect.NewResponse(resp), f.do(r.Msg.GetStore(), false, []string{r.Msg.GetName()}, func() (err error) {
		resp.Info, err = f.stat(r.Msg.GetName())
		return err
	})
}

func (f *fake) ReadDir(_ context.Context, r *connect.Request[runnerv1.ReadDirRequest]) (*connect.Response[runnerv1.ReadDirResponse], error) {
	name, resp := r.Msg.GetName(), &runnerv1.ReadDirResponse{}
	return connect.NewResponse(resp), f.do(r.Msg.GetStore(), false, []string{name}, func() error {
		if fi, err := f.stat(name); err != nil || !fi.GetDir() {
			return cmp.Or(err, status(connect.CodeFailedPrecondition, name))
		}
		resp.Entries = f.list(name)
		if f.reversed.Load() {
			slices.Reverse(resp.Entries)
		}
		return nil
	})
}

func (f *fake) Read(_ context.Context, r *connect.Request[runnerv1.ReadRequest], s *connect.ServerStream[runnerv1.ReadResponse]) error {
	name, data := r.Msg.GetName(), []byte(nil)
	err := f.do(r.Msg.GetStore(), false, []string{name}, func() error {
		if fi, err := f.stat(name); err != nil || fi.GetDir() {
			return cmp.Or(err, status(connect.CodeFailedPrecondition, name))
		}
		data = f.files[name]
		return nil
	})
	for err == nil && len(data) > 0 {
		n := min(7, len(data))
		err = s.Send(&runnerv1.ReadResponse{Data: data[:n]})
		data = data[n:]
	}
	return err
}

// writable reports why no file can be written at name.
func (f *fake) writable(name string) error {
	if fi, err := f.stat(path.Dir(name)); err != nil || !fi.GetDir() || f.dirs[name] {
		return cmp.Or(err, status(connect.CodeFailedPrecondition, name))
	}
	return nil
}

func (f *fake) Write(_ context.Context, s *connect.ClientStream[runnerv1.WriteRequest]) (*connect.Response[runnerv1.WriteResponse], error) {
	if !s.Receive() || s.Msg().GetHeader() == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("a write starts with its header"))
	}
	h := s.Msg().GetHeader()
	if err := f.do(h.GetStore(), true, []string{h.GetName()}, func() error { return f.writable(h.GetName()) }); err != nil {
		return nil, err
	}
	data := []byte{}
	for s.Receive() {
		data = append(data, s.Msg().GetData()...)
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.writable(h.GetName()); err != nil {
		return nil, err
	}
	f.files[h.GetName()] = data
	return connect.NewResponse(&runnerv1.WriteResponse{}), nil
}

func (f *fake) Mkdir(_ context.Context, r *connect.Request[runnerv1.MkdirRequest]) (*connect.Response[runnerv1.MkdirResponse], error) {
	name := r.Msg.GetName()
	return connect.NewResponse(&runnerv1.MkdirResponse{}), f.do(r.Msg.GetStore(), true, []string{name}, func() error {
		elems := strings.Split(name, "/")
		for i := range elems {
			p := strings.Join(elems[:i+1], "/")
			if _, ok := f.files[p]; ok {
				return status(connect.CodeAlreadyExists, name)
			}
			f.dirs[p] = true
		}
		return nil
	})
}

func (f *fake) Remove(_ context.Context, r *connect.Request[runnerv1.RemoveRequest]) (*connect.Response[runnerv1.RemoveResponse], error) {
	name := r.Msg.GetName()
	return connect.NewResponse(&runnerv1.RemoveResponse{}), f.do(r.Msg.GetStore(), true, []string{name}, func() error {
		if fi, err := f.stat(name); err != nil || fi.GetDir() && len(f.list(name)) > 0 {
			return cmp.Or(err, status(connect.CodeFailedPrecondition, name))
		}
		delete(f.files, name)
		delete(f.dirs, name)
		return nil
	})
}

// Rename never replaces a folder, and never moves a folder into itself.
func (f *fake) Rename(_ context.Context, r *connect.Request[runnerv1.RenameRequest]) (*connect.Response[runnerv1.RenameResponse], error) {
	old, name := r.Msg.GetOldName(), r.Msg.GetNewName()
	return connect.NewResponse(&runnerv1.RenameResponse{}), f.do(r.Msg.GetStore(), true, []string{old, name}, func() error {
		fi, err := f.stat(old)
		if err != nil {
			return err
		}
		if _, onFile := f.files[name]; fi.GetDir() && onFile {
			return status(connect.CodeFailedPrecondition, name)
		}
		if fi.GetDir() && strings.HasPrefix(name, old+"/") {
			return status(connect.CodeInvalidArgument, name)
		}
		if err := f.writable(name); err != nil {
			return err
		}
		move(f.files, old, name)
		move(f.dirs, old, name)
		return nil
	})
}

func move[V any](m map[string]V, old, name string) {
	moved := map[string]V{}
	for k, v := range m {
		if k == old || strings.HasPrefix(k, old+"/") {
			moved[name+strings.TrimPrefix(k, old)] = v
			delete(m, k)
		}
	}
	maps.Copy(m, moved)
}
