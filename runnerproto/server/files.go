package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"syscall"
	"unicode"

	"connectrpc.com/connect"

	"aicoded.dev/framework/runnerproto/runnerv1"
)

// maxFileSize is MaxFileSize, lowered by tests.
var maxFileSize int64 = MaxFileSize

const (
	fileChunk = 256 << 10
	tmpPrefix = ".aicoded-tmp-"
	// maxName and maxElem bound a file name in a store and each of its elements, in bytes.
	maxName = 1024
	maxElem = 255
)

// fileService serves the app's declared file stores through files.
type fileService struct {
	stores []string
	files  Files
}

// check refuses a store the app does not declare and a name that is not a valid file name in a
// store. A change may not target the store itself.
func (s *fileService) check(store, name string, change bool) error {
	if !slices.Contains(s.stores, store) {
		return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("the file store %s is not declared in aicoded.yaml", quote(store)))
	}
	if !validName(name) || change && name == "." {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("E-FILE-001: %s is not a valid file name in a store", quote(name)))
	}
	return nil
}

// validName reports whether name is a valid file name in a store: a path that fs.ValidPath
// accepts, of at most maxName bytes, whose elements are at most maxElem bytes, hold no control
// character and do not start with tmpPrefix.
func validName(name string) bool {
	if len(name) > maxName || !fs.ValidPath(name) {
		return false
	}
	return !slices.ContainsFunc(strings.Split(name, "/"), func(e string) bool {
		return len(e) > maxElem || strings.HasPrefix(e, tmpPrefix) || strings.ContainsFunc(e, unicode.IsControl)
	})
}

// fileError turns err, an error of Files from op on name, into the app's error. Its message
// names the file as the app named it and never repeats what Files said. Something in the way of
// a write, rename or removal is FAILED_PRECONDITION.
func fileError(op, name string, err error) error {
	if ce := (*connect.Error)(nil); errors.As(err, &ce) {
		return ce
	}
	code, cause := fileCause(err)
	return connect.NewError(code, &fs.PathError{Op: op, Path: name, Err: cause})
}

// fileCause returns the code of err and the cause the app may read: the errno or fs error err
// wraps when it is one that fileCode maps to a file code, and "internal error" for any other,
// such as a provider's failure to reach its storage.
func fileCause(err error) (connect.Code, error) {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		if code := fileCode(errno); code != connect.CodeInternal {
			return code, errno
		}
	} else {
		for _, known := range []error{fs.ErrNotExist, fs.ErrExist, fs.ErrPermission} {
			if errors.Is(err, known) {
				return fileCode(known), known
			}
		}
	}
	return connect.CodeInternal, errors.New("internal error")
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

func fileInfo(name string, fi FileInfo) *runnerv1.FileInfo {
	out := &runnerv1.FileInfo{Name: name, Size: fi.Size, Dir: fi.Dir}
	if !fi.ModTime.IsZero() {
		out.ModTimeUnixNano = fi.ModTime.UnixNano()
	}
	return out
}

// entry reports whether name, from a folder listing, is an entry the app may see: one element
// of a valid file name, which no file of the provider's own is.
func entry(name string) bool {
	return name != "." && !strings.Contains(name, "/") && validName(name)
}

func (s *fileService) Stat(ctx context.Context, req *connect.Request[runnerv1.StatRequest]) (*connect.Response[runnerv1.StatResponse], error) {
	store, name := req.Msg.GetStore(), req.Msg.GetName()
	if err := s.check(store, name, false); err != nil {
		return nil, err
	}
	fi, err := s.files.Stat(ctx, store, name)
	if err != nil {
		return nil, fileError("stat", name, err)
	}
	return connect.NewResponse(&runnerv1.StatResponse{Info: fileInfo(path.Base(name), fi)}), nil
}

func (s *fileService) ReadDir(ctx context.Context, req *connect.Request[runnerv1.ReadDirRequest]) (*connect.Response[runnerv1.ReadDirResponse], error) {
	store, name := req.Msg.GetStore(), req.Msg.GetName()
	if err := s.check(store, name, false); err != nil {
		return nil, err
	}
	entries, err := s.files.ReadDir(ctx, store, name)
	if err != nil {
		return nil, fileError("readdir", name, err)
	}
	out := []*runnerv1.FileInfo{}
	for _, e := range entries {
		if entry(e.Name) {
			out = append(out, fileInfo(e.Name, e))
		}
	}
	slices.SortFunc(out, func(a, b *runnerv1.FileInfo) int { return strings.Compare(a.GetName(), b.GetName()) })
	return connect.NewResponse(&runnerv1.ReadDirResponse{Entries: out}), nil
}

func (s *fileService) Read(ctx context.Context, req *connect.Request[runnerv1.ReadRequest], stream *connect.ServerStream[runnerv1.ReadResponse]) error {
	store, name := req.Msg.GetStore(), req.Msg.GetName()
	if err := s.check(store, name, false); err != nil {
		return err
	}
	fi, err := s.files.Stat(ctx, store, name)
	if err != nil {
		return fileError("read", name, err)
	}
	if fi.Dir {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("%s is a folder", quote(name)))
	}
	f, err := s.files.Read(ctx, store, name)
	if err != nil {
		return fileError("read", name, err)
	}
	defer f.Close()
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
			return fileError("read", name, err)
		}
	}
}

func (s *fileService) Write(ctx context.Context, stream *connect.ClientStream[runnerv1.WriteRequest]) (*connect.Response[runnerv1.WriteResponse], error) {
	if !stream.Receive() || stream.Msg().GetHeader() == nil {
		if err := stream.Err(); err != nil {
			return nil, err
		}
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("a write starts with its header"))
	}
	h := stream.Msg().GetHeader()
	store, name := h.GetStore(), h.GetName()
	if err := s.check(store, name, true); err != nil {
		return nil, err
	}
	data := &writeData{stream: stream, name: name, max: maxFileSize}
	fi, err := s.files.Write(ctx, store, name, data)
	if data.err != nil {
		return nil, data.err
	}
	if err != nil {
		return nil, fileError("write", name, err)
	}
	return connect.NewResponse(&runnerv1.WriteResponse{Info: fileInfo(path.Base(name), fi)}), nil
}

// writeData reads the data of a write from its stream. It fails, for good, when the stream
// fails, holds a second header or passes max bytes, and keeps why in err.
type writeData struct {
	stream    *connect.ClientStream[runnerv1.WriteRequest]
	name      string
	max, size int64
	buf       []byte
	done      bool
	err       error
}

func (d *writeData) Read(p []byte) (int, error) {
	for len(d.buf) == 0 {
		switch {
		case d.err != nil:
			return 0, d.err
		case d.done:
			return 0, io.EOF
		case !d.stream.Receive():
			if d.err = d.stream.Err(); d.err == nil {
				d.done = true
			}
			continue
		case d.stream.Msg().GetHeader() != nil:
			d.err = connect.NewError(connect.CodeInvalidArgument, errors.New("a write has one header"))
			continue
		}
		data := d.stream.Msg().GetData()
		if d.size += int64(len(data)); d.size > d.max {
			d.err = connect.NewError(connect.CodeResourceExhausted, fmt.Errorf("E-FILE-002: %s is larger than %d bytes", quote(d.name), d.max))
			continue
		}
		d.buf = data
	}
	n := copy(p, d.buf)
	d.buf = d.buf[n:]
	return n, nil
}

func (s *fileService) Mkdir(ctx context.Context, req *connect.Request[runnerv1.MkdirRequest]) (*connect.Response[runnerv1.MkdirResponse], error) {
	store, name := req.Msg.GetStore(), req.Msg.GetName()
	if err := s.check(store, name, true); err != nil {
		return nil, err
	}
	if err := s.files.Mkdir(ctx, store, name); err != nil {
		if errors.Is(err, fs.ErrExist) || errors.Is(err, syscall.ENOTDIR) {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("a file stands at %s or at one of its parent folders", quote(name)))
		}
		return nil, fileError("mkdir", name, err)
	}
	return connect.NewResponse(&runnerv1.MkdirResponse{}), nil
}

func (s *fileService) Remove(ctx context.Context, req *connect.Request[runnerv1.RemoveRequest]) (*connect.Response[runnerv1.RemoveResponse], error) {
	store, name := req.Msg.GetStore(), req.Msg.GetName()
	if err := s.check(store, name, true); err != nil {
		return nil, err
	}
	if err := s.files.Remove(ctx, store, name); err != nil {
		return nil, fileError("remove", name, err)
	}
	return connect.NewResponse(&runnerv1.RemoveResponse{}), nil
}

func (s *fileService) Rename(ctx context.Context, req *connect.Request[runnerv1.RenameRequest]) (*connect.Response[runnerv1.RenameResponse], error) {
	store, from, to := req.Msg.GetStore(), req.Msg.GetOldName(), req.Msg.GetNewName()
	if err := s.check(store, from, true); err != nil {
		return nil, err
	}
	if err := s.check(store, to, true); err != nil {
		return nil, err
	}
	if fi, err := s.files.Stat(ctx, store, to); err == nil && fi.Dir {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("%s is a folder, and a rename never replaces a folder", quote(to)))
	}
	if err := s.files.Rename(ctx, store, from, to); err != nil {
		return nil, fileError("rename", from+" to "+to, err)
	}
	return connect.NewResponse(&runnerv1.RenameResponse{}), nil
}
