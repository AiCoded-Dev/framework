package filestore

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"

	"connectrpc.com/connect"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
	"aicoded.dev/framework/telemetry"
)

const chunk = 256 << 10

// Store is one file store. It is an fs.FS, so it works with fs.ReadFile, fs.WalkDir, fs.Glob and
// template.ParseFS.
type Store struct {
	ctx    context.Context
	name   string
	client runnerv1connect.FilesServiceClient
}

var (
	_ fs.StatFS     = (*Store)(nil)
	_ fs.ReadDirFS  = (*Store)(nil)
	_ fs.ReadFileFS = (*Store)(nil)
)

// Open returns the store name. Its operations keep the values of ctx but not its cancellation,
// so a store opened at start-up serves every request. Open fails with E-MAN-011 when
// aicoded.yaml does not declare filestore:<name>.
func Open(ctx context.Context, name string) (*Store, error) {
	s := runner.From(ctx)
	if s == nil || s.Files == nil {
		return nil, runner.ErrNoRunner
	}
	st := &Store{ctx: context.WithoutCancel(ctx), name: name, client: s.Files}
	_, err := st.client.Stat(ctx, connect.NewRequest(&runnerv1.StatRequest{Store: name, Name: "."}))
	if connect.CodeOf(err) == connect.CodePermissionDenied {
		return nil, errs.New("E-MAN-011", fmt.Sprintf("the file store %q is not declared", name),
			fmt.Sprintf("add - source: filestore:%s with its classes to data in aicoded.yaml", name))
	}
	if err != nil {
		return nil, fmt.Errorf("filestore: open %q: %w", name, err)
	}
	return st, nil
}

func (s *Store) span(op string) (context.Context, *telemetry.Span) {
	ctx, span := telemetry.Start(s.ctx, "filestore."+op)
	span.SetAttr("store", s.name)
	return ctx, span
}

// Stat returns the file's information.
func (s *Store) Stat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, badName("stat", name)
	}
	ctx, span := s.span("stat")
	defer span.End()
	resp, err := s.client.Stat(ctx, connect.NewRequest(&runnerv1.StatRequest{Store: s.name, Name: name}))
	if err != nil {
		span.RecordError(err)
		return nil, pathErr("stat", name, mapErr(err))
	}
	return info{resp.Msg.GetInfo()}, nil
}

// ReadDir returns the folder's entries sorted by name.
func (s *Store) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, badName("readdir", name)
	}
	ctx, span := s.span("readdir")
	defer span.End()
	resp, err := s.client.ReadDir(ctx, connect.NewRequest(&runnerv1.ReadDirRequest{Store: s.name, Name: name}))
	if err != nil {
		span.RecordError(err)
		return nil, pathErr("readdir", name, mapErr(err))
	}
	out := make([]fs.DirEntry, len(resp.Msg.GetEntries()))
	for i, e := range resp.Msg.GetEntries() {
		out[i] = entry{info{e}}
	}
	slices.SortFunc(out, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return out, nil
}

// Open opens the file or folder name for reading.
func (s *Store) Open(name string) (fs.File, error) {
	fi, err := s.Stat(name)
	if err != nil {
		var pe *fs.PathError
		if errors.As(err, &pe) {
			pe.Op = "open"
		}
		return nil, err
	}
	if fi.IsDir() {
		return &dir{info: fi.(info), s: s, name: name}, nil
	}
	return &file{info: fi.(info), s: s, name: name}, nil
}

// ReadFile returns the whole file.
func (s *Store) ReadFile(name string) ([]byte, error) {
	f, err := s.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, ok := f.(*dir); ok {
		return nil, pathErr("read", name, fs.ErrInvalid)
	}
	return io.ReadAll(f)
}

// Create starts writing the file name. The file replaces any file of that name when Close
// succeeds; until then, and after a failed Close, the store is unchanged. Always call Close.
// The folder of name must exist.
func (s *Store) Create(name string) (io.WriteCloser, error) {
	if !fs.ValidPath(name) || name == "." {
		return nil, badName("create", name)
	}
	ctx, span := s.span("write")
	ctx, cancel := context.WithCancel(ctx)
	stream := s.client.Write(ctx)
	header := &runnerv1.WriteRequest{Part: &runnerv1.WriteRequest_Header{Header: &runnerv1.WriteHeader{Store: s.name, Name: name}}}
	if err := stream.Send(header); err != nil {
		_, rerr := stream.CloseAndReceive()
		cancel()
		err = cmp.Or(rerr, err)
		span.RecordError(err)
		span.End()
		return nil, pathErr("create", name, mapErr(err))
	}
	return &writer{name: name, stream: stream, cancel: cancel, span: span}, nil
}

// WriteFile writes data as the file name, replacing any file of that name.
func (s *Store) WriteFile(name string, data []byte) error {
	w, err := s.Create(name)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

// Mkdir creates the folder name and any missing parents.
func (s *Store) Mkdir(name string) error {
	return s.change("mkdir", name, func(ctx context.Context) error {
		_, err := s.client.Mkdir(ctx, connect.NewRequest(&runnerv1.MkdirRequest{Store: s.name, Name: name}))
		return err
	})
}

// Remove removes the file or empty folder name.
func (s *Store) Remove(name string) error {
	return s.change("remove", name, func(ctx context.Context) error {
		_, err := s.client.Remove(ctx, connect.NewRequest(&runnerv1.RemoveRequest{Store: s.name, Name: name}))
		return err
	})
}

// Rename moves oldName to newName, replacing a file of that name. A folder at newName is
// never replaced.
func (s *Store) Rename(oldName, newName string) error {
	if !fs.ValidPath(newName) || newName == "." {
		return badName("rename", newName)
	}
	return s.change("rename", oldName, func(ctx context.Context) error {
		_, err := s.client.Rename(ctx, connect.NewRequest(&runnerv1.RenameRequest{Store: s.name, OldName: oldName, NewName: newName}))
		return err
	})
}

func (s *Store) change(op, name string, call func(context.Context) error) error {
	if !fs.ValidPath(name) || name == "." {
		return badName(op, name)
	}
	ctx, span := s.span(op)
	defer span.End()
	if err := call(ctx); err != nil {
		span.RecordError(err)
		return pathErr(op, name, mapErr(err))
	}
	return nil
}

func pathErr(op, name string, err error) error { return &fs.PathError{Op: op, Path: name, Err: err} }

const badNameFix = "use a relative path inside the store, such as invoices/2026/42.pdf"

var errBadName = errs.New("E-FILE-001", "not a valid file name in a store", badNameFix)

func badName(op, name string) error {
	return pathErr(op, name, fmt.Errorf("%w: %w", fs.ErrInvalid, errBadName))
}

// mapErr turns the runner's status codes into the io/fs errors callers test with errors.Is.
// A name the runner refuses is E-FILE-001, as one the client refuses, and a file it refuses as
// too large E-FILE-002.
func mapErr(err error) error {
	var ce *connect.Error
	if !errors.As(err, &ce) {
		return err
	}
	switch ce.Code() {
	case connect.CodeNotFound:
		return fs.ErrNotExist
	case connect.CodeAlreadyExists:
		return fs.ErrExist
	case connect.CodePermissionDenied:
		return fs.ErrPermission
	case connect.CodeInvalidArgument:
		return fmt.Errorf("%w: %w", fs.ErrInvalid, errs.New("E-FILE-001", strings.TrimPrefix(ce.Message(), "E-FILE-001: "), badNameFix))
	case connect.CodeResourceExhausted:
		return fmt.Errorf("%w: %w", fs.ErrInvalid, errs.New("E-FILE-002", strings.TrimPrefix(ce.Message(), "E-FILE-002: "),
			"split the content into several files, or keep large media outside the app"))
	case connect.CodeFailedPrecondition:
		return fmt.Errorf("%w: %s", fs.ErrInvalid, ce.Message())
	}
	return err
}
