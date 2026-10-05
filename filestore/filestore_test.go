package filestore

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"testing/iotest"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
)

func TestStoreIsAnFS(t *testing.T) {
	ctx, _ := fakeRunner(t)
	st, err := Open(ctx, "docs")
	require.NoError(t, err)
	require.NoError(t, st.Mkdir("dir/sub"))
	require.NoError(t, st.WriteFile("a.txt", []byte("alpha")))
	require.NoError(t, st.WriteFile("dir/b.txt", []byte("beta")))
	require.NoError(t, fstest.TestFS(st, "a.txt", "dir/b.txt", "dir/sub"))
}

func TestReaderContract(t *testing.T) {
	ctx, _ := fakeRunner(t)
	st, err := Open(ctx, "docs")
	require.NoError(t, err)
	content := strings.Repeat("0123456789", 1000)
	w, err := st.Create("big.txt")
	require.NoError(t, err)
	_, err = io.Copy(w, strings.NewReader(content))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	f, err := st.Open("big.txt")
	require.NoError(t, err)
	defer f.Close()
	require.NoError(t, iotest.TestReader(f, []byte(content)))
}

func TestErrors(t *testing.T) {
	ctx, calls := fakeRunner(t)
	_, err := Open(ctx, "invoices")
	assert.Equal(t, "E-MAN-011", errs.Code(err))
	_, err = Open(context.Background(), "docs")
	assert.Equal(t, "E-RUN-004", errs.Code(err))

	st, err := Open(ctx, "docs")
	require.NoError(t, err)
	_, err = st.Stat("missing.txt")
	require.ErrorIs(t, err, fs.ErrNotExist)
	var pe *fs.PathError
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, "missing.txt", pe.Path)

	before := calls()
	for _, name := range []string{"../x", "/abs", "a//b"} {
		_, err := st.ReadFile(name)
		require.ErrorIs(t, err, fs.ErrInvalid, name)
		assert.Equal(t, "E-FILE-001", errs.Code(err), name)
		require.ErrorIs(t, st.WriteFile(name, nil), fs.ErrInvalid, name)
		require.ErrorIs(t, st.Mkdir(name), fs.ErrInvalid, name)
		require.ErrorIs(t, st.Rename("a.txt", name), fs.ErrInvalid, name)
	}
	for _, err := range []error{st.Remove("."), st.Rename(".", "x"), st.Rename("x", "."), st.WriteFile(".", nil)} {
		assert.Equal(t, "E-FILE-001", errs.Code(err))
	}
	assert.Equal(t, before, calls(), "a bad name never reaches the runner")
	_, err = st.Stat("a/.aicoded-tmp-x")
	require.ErrorIs(t, err, fs.ErrInvalid)
	assert.Equal(t, "E-FILE-001", errs.Code(err), "a name the runner refuses")

	require.NoError(t, st.WriteFile("x.txt", []byte("x")))
	require.ErrorIs(t, st.Mkdir("x.txt"), fs.ErrExist)
	require.ErrorIs(t, st.Remove("nope"), fs.ErrNotExist)
	_, err = st.ReadFile(".")
	require.ErrorIs(t, err, fs.ErrInvalid, "a folder is not read as a file")
	f, err := st.Open("x.txt")
	require.NoError(t, err)
	defer f.Close()
	require.NoError(t, st.Remove("x.txt"))
	_, err = f.Read(make([]byte, 8))
	assert.ErrorIs(t, err, fs.ErrNotExist, "a file removed after Open")
}

func TestReadDirSorts(t *testing.T) {
	ctx, f := newFake(t)
	f.reversed.Store(true)
	st, err := Open(ctx, "docs")
	require.NoError(t, err)
	for _, name := range []string{"a", "b", "c"} {
		require.NoError(t, st.WriteFile(name, nil))
	}
	entries, err := st.ReadDir(".")
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Equal(t, []string{"a", "b", "c"}, names)
}

func TestReadAfterClose(t *testing.T) {
	ctx, _ := fakeRunner(t)
	st, err := Open(ctx, "docs")
	require.NoError(t, err)
	require.NoError(t, st.WriteFile("x.txt", []byte("0123456789")))
	for _, first := range []int{0, 3} { // closed before reading, and in the middle
		f, err := st.Open("x.txt")
		require.NoError(t, err)
		_, err = f.Read(make([]byte, first))
		require.NoError(t, err)
		require.NoError(t, f.Close())
		_, err = f.Read(make([]byte, 3))
		require.ErrorIs(t, err, fs.ErrClosed)
		var pe *fs.PathError
		assert.ErrorAs(t, err, &pe)
	}
}

func TestCreateCommitsOnClose(t *testing.T) {
	ctx, _ := fakeRunner(t)
	st, err := Open(ctx, "docs")
	require.NoError(t, err)
	w, err := st.Create("c.txt")
	require.NoError(t, err)
	_, err = w.Write([]byte("draft"))
	require.NoError(t, err)
	_, err = st.Stat("c.txt")
	require.ErrorIs(t, err, fs.ErrNotExist, "nothing is visible before Close")
	require.NoError(t, w.Close())
	got, err := st.ReadFile("c.txt")
	require.NoError(t, err)
	assert.Equal(t, "draft", string(got))

	require.ErrorIs(t, st.WriteFile("nope/d.txt", make([]byte, 16*chunk)), fs.ErrNotExist)
	entries, err := st.ReadDir(".")
	require.NoError(t, err)
	assert.Len(t, entries, 1, "a failed write leaves the store unchanged")
}

func TestChanges(t *testing.T) {
	ctx, _ := fakeRunner(t)
	st, err := Open(ctx, "docs")
	require.NoError(t, err)
	require.NoError(t, st.Mkdir("a/b"))
	require.NoError(t, st.Mkdir("a"), "an existing folder is not an error")
	require.NoError(t, st.WriteFile("a/b/x.txt", []byte("x")))
	require.NoError(t, st.Rename("a/b/x.txt", "a/y.txt"))
	require.NoError(t, st.WriteFile("a/b/z.txt", []byte("z")))
	require.NoError(t, st.Rename("a/b/z.txt", "a/y.txt"), "a file is replaced")
	require.ErrorIs(t, st.Rename("a/y.txt", "a/b"), fs.ErrInvalid, "a folder is never replaced")
	require.ErrorIs(t, st.Remove("a"), fs.ErrInvalid, "a folder that is not empty stays")
	require.NoError(t, st.Remove("a/b"))
	require.NoError(t, st.Rename("a", "c"))
	got, err := st.ReadFile("c/y.txt")
	require.NoError(t, err)
	assert.Equal(t, "z", string(got))
}

func TestMapErr(t *testing.T) {
	for code, want := range map[connect.Code]error{
		connect.CodeNotFound:           fs.ErrNotExist,
		connect.CodeAlreadyExists:      fs.ErrExist,
		connect.CodePermissionDenied:   fs.ErrPermission,
		connect.CodeInvalidArgument:    fs.ErrInvalid,
		connect.CodeFailedPrecondition: fs.ErrInvalid,
		connect.CodeResourceExhausted:  fs.ErrInvalid,
	} {
		require.ErrorIs(t, mapErr(connect.NewError(code, errors.New("x"))), want, code.String())
	}
	assert.Equal(t, "E-FILE-001", errs.Code(mapErr(connect.NewError(connect.CodeInvalidArgument, errors.New("x")))))
	var tooLarge *errs.Error
	require.ErrorAs(t, mapErr(connect.NewError(connect.CodeResourceExhausted, errors.New(`E-FILE-002: "a.bin" is too large`))), &tooLarge)
	assert.Equal(t, "E-FILE-002", tooLarge.Code)
	assert.Equal(t, `"a.bin" is too large`, tooLarge.Msg)
	unavailable := connect.NewError(connect.CodeUnavailable, errors.New("x"))
	assert.Equal(t, unavailable, mapErr(unavailable))
}

func TestSpans(t *testing.T) {
	ctx, f := newFake(t)
	st, err := Open(ctx, "docs")
	require.NoError(t, err)
	require.NoError(t, st.WriteFile("private.txt", []byte("x")))
	_, err = st.ReadFile("private.txt")
	require.NoError(t, err)
	_, err = st.Stat("private-missing.txt")
	require.Error(t, err)
	require.NoError(t, st.Rename("private.txt", "private2.txt"))

	var names []string
	for _, s := range f.spans {
		names = append(names, s.GetName())
		assert.Equal(t, map[string]string{"store": "docs"}, s.GetAttributes())
		assert.NotContains(t, s.GetError(), "private")
	}
	assert.Equal(t, []string{"filestore.write", "filestore.stat", "filestore.read", "filestore.stat", "filestore.rename"}, names)
	assert.Equal(t, "not_found", f.spans[3].GetError())
}
