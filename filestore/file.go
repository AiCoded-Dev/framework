package filestore

import (
	"cmp"
	"context"
	"io"
	"io/fs"
	"time"

	"connectrpc.com/connect"

	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/telemetry"
)

type info struct{ f *runnerv1.FileInfo }

func (i info) Name() string       { return i.f.GetName() }
func (i info) Size() int64        { return i.f.GetSize() }
func (i info) ModTime() time.Time { return time.Unix(0, i.f.GetModTimeUnixNano()) }
func (i info) IsDir() bool        { return i.f.GetDir() }
func (i info) Sys() any           { return nil }

func (i info) Mode() fs.FileMode {
	if i.f.GetDir() {
		return fs.ModeDir | 0o755
	}
	return 0o644
}

type entry struct{ info }

func (e entry) Type() fs.FileMode          { return e.Mode().Type() }
func (e entry) Info() (fs.FileInfo, error) { return e.info, nil }

// file reads a file as a stream; Read follows the io.Reader contract.
type file struct {
	info   info
	s      *Store
	name   string
	stream *connect.ServerStreamForClient[runnerv1.ReadResponse]
	cancel context.CancelFunc
	span   *telemetry.Span
	buf    []byte
	err    error
}

func (f *file) Stat() (fs.FileInfo, error) { return f.info, nil }

func (f *file) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for len(f.buf) == 0 {
		if f.err != nil {
			return 0, f.err
		}
		if f.stream == nil {
			if err := f.open(); err != nil {
				f.fail(err)
				continue
			}
		}
		if !f.stream.Receive() {
			f.fail(f.stream.Err())
			continue
		}
		f.buf = f.stream.Msg().GetData()
	}
	n := copy(p, f.buf)
	f.buf = f.buf[n:]
	return n, nil
}

func (f *file) open() error {
	ctx, span := f.s.span("read")
	ctx, cancel := context.WithCancel(ctx)
	f.span = span
	stream, err := f.s.client.Read(ctx, connect.NewRequest(&runnerv1.ReadRequest{Store: f.s.name, Name: f.name}))
	if err != nil {
		cancel()
		return err
	}
	f.stream, f.cancel = stream, cancel
	return nil
}

// fail ends the read with err, or with io.EOF when err is nil.
func (f *file) fail(err error) {
	f.err = io.EOF
	if err != nil {
		f.span.RecordError(err)
		f.err = pathErr("read", f.name, mapErr(err))
	}
	f.span.End()
}

func (f *file) Close() error {
	f.buf, f.err = nil, pathErr("read", f.name, fs.ErrClosed)
	if f.stream == nil {
		return nil
	}
	defer f.span.End()
	defer f.cancel()
	return f.stream.Close()
}

// dir is an open folder.
type dir struct {
	info    info
	s       *Store
	name    string
	entries []fs.DirEntry
	loaded  bool
	off     int
}

func (d *dir) Stat() (fs.FileInfo, error) { return d.info, nil }
func (d *dir) Read([]byte) (int, error)   { return 0, pathErr("read", d.name, fs.ErrInvalid) }
func (d *dir) Close() error               { return nil }

func (d *dir) ReadDir(n int) ([]fs.DirEntry, error) {
	if !d.loaded {
		es, err := d.s.ReadDir(d.name)
		if err != nil {
			return nil, err
		}
		d.entries, d.loaded = es, true
	}
	rest := d.entries[d.off:]
	if n <= 0 {
		d.off = len(d.entries)
		return rest, nil
	}
	if len(rest) == 0 {
		return nil, io.EOF
	}
	n = min(n, len(rest))
	d.off += n
	return rest[:n], nil
}

// writer streams a file to the runner; Close commits it.
type writer struct {
	name   string
	stream *connect.ClientStreamForClient[runnerv1.WriteRequest, runnerv1.WriteResponse]
	cancel context.CancelFunc
	span   *telemetry.Span
	err    error
	closed bool
}

func (w *writer) Write(p []byte) (int, error) {
	if w.closed {
		return 0, pathErr("write", w.name, fs.ErrClosed)
	}
	if w.err != nil {
		return 0, w.err
	}
	for off := 0; off < len(p); off += chunk {
		data := p[off:min(off+chunk, len(p))]
		if err := w.stream.Send(&runnerv1.WriteRequest{Part: &runnerv1.WriteRequest_Data{Data: data}}); err != nil {
			_, rerr := w.stream.CloseAndReceive()
			w.cancel()
			w.fail(cmp.Or(rerr, err))
			return off, w.err
		}
	}
	return len(p), nil
}

func (w *writer) fail(err error) {
	w.span.RecordError(err)
	w.err = pathErr("write", w.name, mapErr(err))
}

func (w *writer) Close() error {
	if w.closed {
		return w.err
	}
	w.closed = true
	defer w.span.End()
	defer w.cancel()
	if w.err != nil {
		return w.err
	}
	if _, err := w.stream.CloseAndReceive(); err != nil {
		w.fail(err)
	}
	return w.err
}
