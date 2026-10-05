package dev

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"connectrpc.com/connect"

	"aicoded.dev/framework/runnerproto/runnerv1"
)

// maxFileSize bounds one file in a store.
var maxFileSize int64 = 1 << 30

const (
	fileChunk = 256 << 10
	tmpPrefix = ".aicoded-tmp-"
)

// fileService serves an app's declared file stores. Each store is a folder opened as an
// os.Root, so no name reaches outside it, through ".." or through a symbolic link.
type fileService struct {
	stores  map[string]*os.Root
	maxSize int64
}

func newFileService(dir string, names []string) (*fileService, error) {
	s := &fileService{stores: map[string]*os.Root{}, maxSize: maxFileSize}
	for _, n := range names {
		p := filepath.Join(dir, n)
		if err := os.MkdirAll(p, 0o700); err != nil {
			s.close()
			return nil, err
		}
		r, err := os.OpenRoot(p)
		if err != nil {
			s.close()
			return nil, err
		}
		s.stores[n] = r
		if err := sweep(r); err != nil {
			s.close()
			return nil, err
		}
	}
	return s, nil
}

// sweep removes the temporary files that writes cut short by a crash left anywhere in the store.
func sweep(r *os.Root) error {
	return fs.WalkDir(r.FS(), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && isTemp(d.Name()) {
			return r.Remove(name)
		}
		return nil
	})
}

func (s *fileService) close() {
	for _, r := range s.stores {
		_ = r.Close()
	}
}

func (s *fileService) store(name string) (*os.Root, error) {
	if r := s.stores[name]; r != nil {
		return r, nil
	}
	return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("the file store %q is not declared in aicoded.yaml", name))
}

// open returns the store and checks name. A change may not target the store itself.
func (s *fileService) open(store, name string, change bool) (*os.Root, error) {
	r, err := s.store(store)
	if err != nil {
		return nil, err
	}
	if !fs.ValidPath(name) || change && name == "." || slices.ContainsFunc(strings.Split(name, "/"), isTemp) {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("E-FILE-001: %q is not a valid file name in a store", name))
	}
	return r, nil
}

func isTemp(elem string) bool { return strings.HasPrefix(elem, tmpPrefix) }

// fileError maps err to a status. Its message names the file as the app named it, never by its
// path on the host or by a temporary name. Something in the way of a write, rename or removal is
// FAILED_PRECONDITION; only Mkdir reports ALREADY_EXISTS.
func fileError(name string, err error) error {
	var ce *connect.Error
	if errors.As(err, &ce) {
		return err
	}
	var pe *fs.PathError
	var le *os.LinkError
	switch {
	case errors.As(err, &pe):
		err = &fs.PathError{Op: pe.Op, Path: name, Err: pe.Err}
	case errors.As(err, &le):
		err = &fs.PathError{Op: le.Op, Path: name, Err: le.Err}
	}
	return connect.NewError(fileCode(err), err)
}

func fileCode(err error) connect.Code {
	switch {
	case errors.Is(err, syscall.EINVAL), errors.Is(err, syscall.ENAMETOOLONG):
		return connect.CodeInvalidArgument
	case errors.Is(err, fs.ErrExist), errors.Is(err, syscall.ENOTEMPTY), errors.Is(err, syscall.EISDIR), errors.Is(err, syscall.ENOTDIR):
		return connect.CodeFailedPrecondition
	case errors.Is(err, fs.ErrNotExist):
		return connect.CodeNotFound
	case errors.Is(err, fs.ErrPermission):
		return connect.CodePermissionDenied
	}
	return connect.CodeInternal
}

func fileInfo(name string, fi fs.FileInfo) *runnerv1.FileInfo {
	return &runnerv1.FileInfo{Name: name, Size: fi.Size(), Dir: fi.IsDir(), ModTimeUnixNano: fi.ModTime().UnixNano()}
}

func (s *fileService) Stat(_ context.Context, req *connect.Request[runnerv1.StatRequest]) (*connect.Response[runnerv1.StatResponse], error) {
	r, err := s.open(req.Msg.GetStore(), req.Msg.GetName(), false)
	if err != nil {
		return nil, err
	}
	fi, err := r.Stat(req.Msg.GetName())
	if err != nil {
		return nil, fileError(req.Msg.GetName(), err)
	}
	return connect.NewResponse(&runnerv1.StatResponse{Info: fileInfo(path.Base(req.Msg.GetName()), fi)}), nil
}

func (s *fileService) ReadDir(_ context.Context, req *connect.Request[runnerv1.ReadDirRequest]) (*connect.Response[runnerv1.ReadDirResponse], error) {
	r, err := s.open(req.Msg.GetStore(), req.Msg.GetName(), false)
	if err != nil {
		return nil, err
	}
	f, err := r.Open(req.Msg.GetName())
	if err != nil {
		return nil, fileError(req.Msg.GetName(), err)
	}
	defer f.Close()
	entries, err := f.ReadDir(-1)
	if err != nil {
		return nil, fileError(req.Msg.GetName(), err)
	}
	out := []*runnerv1.FileInfo{}
	for _, e := range entries {
		if isTemp(e.Name()) {
			continue
		}
		fi, err := e.Info()
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fileError(path.Join(req.Msg.GetName(), e.Name()), err)
		}
		out = append(out, fileInfo(e.Name(), fi))
	}
	slices.SortFunc(out, func(a, b *runnerv1.FileInfo) int { return strings.Compare(a.GetName(), b.GetName()) })
	return connect.NewResponse(&runnerv1.ReadDirResponse{Entries: out}), nil
}

func (s *fileService) Read(_ context.Context, req *connect.Request[runnerv1.ReadRequest], stream *connect.ServerStream[runnerv1.ReadResponse]) error {
	r, err := s.open(req.Msg.GetStore(), req.Msg.GetName(), false)
	if err != nil {
		return err
	}
	f, err := r.Open(req.Msg.GetName())
	if err != nil {
		return fileError(req.Msg.GetName(), err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return fileError(req.Msg.GetName(), err)
	}
	if fi.IsDir() {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("%q is a folder", req.Msg.GetName()))
	}
	buf := make([]byte, fileChunk)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			if err := stream.Send(&runnerv1.ReadResponse{Data: buf[:n]}); err != nil {
				return err
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fileError(req.Msg.GetName(), err)
		}
	}
}

func (s *fileService) Write(_ context.Context, stream *connect.ClientStream[runnerv1.WriteRequest]) (*connect.Response[runnerv1.WriteResponse], error) {
	if !stream.Receive() || stream.Msg().GetHeader() == nil {
		if err := stream.Err(); err != nil {
			return nil, err
		}
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("a write starts with its header"))
	}
	h := stream.Msg().GetHeader()
	r, err := s.open(h.GetStore(), h.GetName(), true)
	if err != nil {
		return nil, err
	}
	name := h.GetName()
	tmp := path.Join(path.Dir(name), tmpPrefix+rand.Text())
	f, err := r.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fileError(name, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = f.Close()
			_ = r.Remove(tmp)
		}
	}()
	var size int64
	for stream.Receive() {
		if stream.Msg().GetHeader() != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("a write has one header"))
		}
		data := stream.Msg().GetData()
		if size += int64(len(data)); size > s.maxSize {
			return nil, connect.NewError(connect.CodeResourceExhausted, fmt.Errorf("E-FILE-002: %q is larger than %d bytes", name, s.maxSize))
		}
		if _, err := f.Write(data); err != nil {
			return nil, fileError(name, err)
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	if err := f.Sync(); err != nil {
		return nil, fileError(name, err)
	}
	if err := f.Close(); err != nil {
		return nil, fileError(name, err)
	}
	if err := r.Rename(tmp, name); err != nil {
		return nil, fileError(name, err)
	}
	committed = true
	fi, err := r.Stat(name)
	if err != nil {
		return nil, fileError(name, err)
	}
	return connect.NewResponse(&runnerv1.WriteResponse{Info: fileInfo(path.Base(name), fi)}), nil
}

func (s *fileService) Mkdir(_ context.Context, req *connect.Request[runnerv1.MkdirRequest]) (*connect.Response[runnerv1.MkdirResponse], error) {
	r, err := s.open(req.Msg.GetStore(), req.Msg.GetName(), true)
	if err != nil {
		return nil, err
	}
	if err := r.MkdirAll(req.Msg.GetName(), 0o700); err != nil {
		if errors.Is(err, fs.ErrExist) || errors.Is(err, syscall.ENOTDIR) {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("a file stands at %q or at one of its parent folders", req.Msg.GetName()))
		}
		return nil, fileError(req.Msg.GetName(), err)
	}
	return connect.NewResponse(&runnerv1.MkdirResponse{}), nil
}

func (s *fileService) Remove(_ context.Context, req *connect.Request[runnerv1.RemoveRequest]) (*connect.Response[runnerv1.RemoveResponse], error) {
	r, err := s.open(req.Msg.GetStore(), req.Msg.GetName(), true)
	if err != nil {
		return nil, err
	}
	if err := r.Remove(req.Msg.GetName()); err != nil {
		return nil, fileError(req.Msg.GetName(), err)
	}
	return connect.NewResponse(&runnerv1.RemoveResponse{}), nil
}

func (s *fileService) Rename(_ context.Context, req *connect.Request[runnerv1.RenameRequest]) (*connect.Response[runnerv1.RenameResponse], error) {
	r, err := s.open(req.Msg.GetStore(), req.Msg.GetOldName(), true)
	if err != nil {
		return nil, err
	}
	if _, err := s.open(req.Msg.GetStore(), req.Msg.GetNewName(), true); err != nil {
		return nil, err
	}
	if fi, err := r.Stat(req.Msg.GetNewName()); err == nil && fi.IsDir() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("%q is a folder, and a rename never replaces a folder", req.Msg.GetNewName()))
	}
	if err := r.Rename(req.Msg.GetOldName(), req.Msg.GetNewName()); err != nil {
		return nil, fileError(req.Msg.GetOldName()+" to "+req.Msg.GetNewName(), err)
	}
	return connect.NewResponse(&runnerv1.RenameResponse{}), nil
}
