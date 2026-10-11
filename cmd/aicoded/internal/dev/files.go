package dev

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"aicoded.dev/framework/runnerproto/server"
)

const tmpPrefix = ".aicoded-tmp-"

// fileStore keeps an app's file stores in folders on disk. Each store is a folder opened as an
// os.Root, so no name reaches outside it, through ".." or through a symbolic link. A write goes
// to a temporary file that is synced and renamed into place.
type fileStore struct {
	roots map[string]*os.Root
}

// newFileStore opens a folder in dir for each store of names, made when missing, and removes the
// temporary files that writes cut short by a crash left in them.
func newFileStore(dir string, names []string) (*fileStore, error) {
	s := &fileStore{roots: map[string]*os.Root{}}
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
		s.roots[n] = r
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

func (s *fileStore) close() {
	for _, r := range s.roots {
		_ = r.Close()
	}
}

func isTemp(elem string) bool { return strings.HasPrefix(elem, tmpPrefix) }

// root returns the folder of store.
func (s *fileStore) root(store string) (*os.Root, error) {
	if r := s.roots[store]; r != nil {
		return r, nil
	}
	return nil, fs.ErrPermission
}

func info(fi fs.FileInfo) server.FileInfo {
	return server.FileInfo{Name: fi.Name(), Size: fi.Size(), Dir: fi.IsDir(), ModTime: fi.ModTime()}
}

func (s *fileStore) Stat(_ context.Context, store, name string) (server.FileInfo, error) {
	r, err := s.root(store)
	if err != nil {
		return server.FileInfo{}, err
	}
	fi, err := r.Stat(name)
	if err != nil {
		return server.FileInfo{}, err
	}
	return info(fi), nil
}

func (s *fileStore) ReadDir(_ context.Context, store, name string) ([]server.FileInfo, error) {
	r, err := s.root(store)
	if err != nil {
		return nil, err
	}
	f, err := r.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries, err := f.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	var out []server.FileInfo
	for _, e := range entries {
		if isTemp(e.Name()) {
			continue
		}
		fi, err := e.Info()
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, info(fi))
	}
	return out, nil
}

func (s *fileStore) Read(_ context.Context, store, name string) (io.ReadCloser, error) {
	r, err := s.root(store)
	if err != nil {
		return nil, err
	}
	f, err := r.Open(name)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err == nil && fi.IsDir() {
		err = syscall.EISDIR
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func (s *fileStore) Write(_ context.Context, store, name string, data io.Reader) (server.FileInfo, error) {
	r, err := s.root(store)
	if err != nil {
		return server.FileInfo{}, err
	}
	tmp := path.Join(path.Dir(name), tmpPrefix+rand.Text())
	f, err := r.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return server.FileInfo{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = f.Close()
			_ = r.Remove(tmp)
		}
	}()
	if _, err := io.Copy(f, data); err != nil {
		return server.FileInfo{}, err
	}
	if err := f.Sync(); err != nil {
		return server.FileInfo{}, err
	}
	if err := f.Close(); err != nil {
		return server.FileInfo{}, err
	}
	if err := r.Rename(tmp, name); err != nil {
		return server.FileInfo{}, err
	}
	committed = true
	fi, err := r.Stat(name)
	if err != nil {
		return server.FileInfo{}, err
	}
	return info(fi), nil
}

func (s *fileStore) Mkdir(_ context.Context, store, name string) error {
	r, err := s.root(store)
	if err != nil {
		return err
	}
	return r.MkdirAll(name, 0o700)
}

func (s *fileStore) Remove(_ context.Context, store, name string) error {
	r, err := s.root(store)
	if err != nil {
		return err
	}
	return r.Remove(name)
}

func (s *fileStore) Rename(_ context.Context, store, oldName, newName string) error {
	r, err := s.root(store)
	if err != nil {
		return err
	}
	return r.Rename(oldName, newName)
}
