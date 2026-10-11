package server

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"strings"
	"syscall"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
)

func header(store, name string) *runnerv1.WriteRequest {
	return &runnerv1.WriteRequest{Part: &runnerv1.WriteRequest_Header{Header: &runnerv1.WriteHeader{Store: store, Name: name}}}
}

func chunk(data string) *runnerv1.WriteRequest {
	return &runnerv1.WriteRequest{Part: &runnerv1.WriteRequest_Data{Data: []byte(data)}}
}

// write sends parts as one write and returns its result.
func write(ctx context.Context, c runnerv1connect.FilesServiceClient, parts ...*runnerv1.WriteRequest) (*runnerv1.WriteResponse, error) {
	s := c.Write(ctx)
	for _, p := range parts {
		if err := s.Send(p); err != nil {
			break
		}
	}
	resp, err := s.CloseAndReceive()
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func putFile(ctx context.Context, c runnerv1connect.FilesServiceClient, name string, chunks ...string) error {
	parts := []*runnerv1.WriteRequest{header("docs", name)}
	for _, d := range chunks {
		parts = append(parts, chunk(d))
	}
	_, err := write(ctx, c, parts...)
	return err
}

func getFile(t *testing.T, c runnerv1connect.FilesServiceClient, name string) (string, error) {
	s, err := c.Read(t.Context(), connect.NewRequest(&runnerv1.ReadRequest{Store: "docs", Name: name}))
	if err != nil {
		return "", err
	}
	var out []byte
	for s.Receive() {
		out = append(out, s.Msg().GetData()...)
	}
	return string(out), s.Err()
}

func stat(ctx context.Context, c runnerv1connect.FilesServiceClient, name string) error {
	_, err := c.Stat(ctx, connect.NewRequest(&runnerv1.StatRequest{Store: "docs", Name: name}))
	return err
}

func mkdir(ctx context.Context, c runnerv1connect.FilesServiceClient, name string) error {
	_, err := c.Mkdir(ctx, connect.NewRequest(&runnerv1.MkdirRequest{Store: "docs", Name: name}))
	return err
}

func remove(ctx context.Context, c runnerv1connect.FilesServiceClient, name string) error {
	_, err := c.Remove(ctx, connect.NewRequest(&runnerv1.RemoveRequest{Store: "docs", Name: name}))
	return err
}

func rename(ctx context.Context, c runnerv1connect.FilesServiceClient, oldName, newName string) error {
	_, err := c.Rename(ctx, connect.NewRequest(&runnerv1.RenameRequest{Store: "docs", OldName: oldName, NewName: newName}))
	return err
}

func TestFiles(t *testing.T) {
	cs, f := serve(t)
	c, ctx := cs.files, t.Context()
	require.NoError(t, mkdir(ctx, c, "a"))
	resp, err := write(ctx, c, header("docs", "a/b.txt"), chunk("hello "), chunk("world"))
	require.NoError(t, err)
	assert.Equal(t, "b.txt", resp.GetInfo().GetName())
	assert.Equal(t, int64(11), resp.GetInfo().GetSize())
	got, err := getFile(t, c, "a/b.txt")
	require.NoError(t, err)
	assert.Equal(t, "hello world", got)
	st, err := c.Stat(ctx, connect.NewRequest(&runnerv1.StatRequest{Store: "docs", Name: "a/b.txt"}))
	require.NoError(t, err)
	assert.Equal(t, "b.txt", st.Msg.GetInfo().GetName())

	for _, planted := range []string{"a/" + tmpPrefix + "x", "a/c\x01d", "a/" + strings.Repeat("e", maxElem+1)} {
		f.files.put("docs", planted, &fakeEntry{})
	}
	require.NoError(t, mkdir(ctx, c, "a/z"))
	list, err := c.ReadDir(ctx, connect.NewRequest(&runnerv1.ReadDirRequest{Store: "docs", Name: "a"}))
	require.NoError(t, err)
	var names []string
	for _, e := range list.Msg.GetEntries() {
		names = append(names, e.GetName())
	}
	assert.Equal(t, []string{"b.txt", "z"}, names, "sorted, and only names the app may use")

	require.NoError(t, rename(ctx, c, "a/b.txt", "a/c.txt"))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(stat(ctx, c, "a/b.txt")))
	require.NoError(t, remove(ctx, c, "a/c.txt"))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(remove(ctx, c, "a/c.txt")))
}

func TestFilesRefuseNames(t *testing.T) {
	cs, f := serve(t)
	c, ctx := cs.files, t.Context()
	for _, name := range []string{"../x", "/etc/passwd", "a/../../x", "", "a//b", "a/", tmpPrefix + "x", "a/" + tmpPrefix + "y",
		"a/" + tmpPrefix + "z/b", "a\x00b", "a\nb", "a/\x7f", strings.Repeat("a", maxElem+1), strings.Repeat("a/", maxName/2) + "a"} {
		for op, err := range map[string]error{
			"stat":   stat(ctx, c, name),
			"write":  putFile(ctx, c, name, "x"),
			"mkdir":  mkdir(ctx, c, name),
			"remove": remove(ctx, c, name),
			"from":   rename(ctx, c, name, "x"),
			"to":     rename(ctx, c, "x", name),
		} {
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "%s %q", op, name)
			require.ErrorContains(t, err, "E-FILE-001", "%s %q", op, name)
		}
		_, err := getFile(t, c, name)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), name)
	}
	for _, err := range []error{putFile(ctx, c, ".", "x"), mkdir(ctx, c, "."), remove(ctx, c, "."), rename(ctx, c, ".", "x"), rename(ctx, c, "x", ".")} {
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "the store itself cannot be changed")
	}
	_, err := c.Stat(ctx, connect.NewRequest(&runnerv1.StatRequest{Store: "other", Name: "."}))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err), "an undeclared store")
	assert.Empty(t, f.files.seen(), "Files never saw a refused call")
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(stat(ctx, c, strings.Repeat("a/", maxName/2-1)+"a")), "a name of 1023 bytes reaches Files")
}

func TestFileWrites(t *testing.T) {
	defer func(old int64) { maxFileSize = old }(maxFileSize)
	maxFileSize = 4
	cs, f := serve(t)
	c, ctx := cs.files, t.Context()
	require.NoError(t, putFile(ctx, c, "small.txt", "12", "34"))
	err := putFile(ctx, c, "big.txt", "12", "345")
	assert.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(err))
	require.ErrorContains(t, err, "E-FILE-002")
	_, err = write(ctx, c, header("docs", "two.txt"), chunk("1"), header("docs", "x"))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "a second header")
	_, err = write(ctx, c, chunk("1"))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "no header")
	_, err = write(ctx, c)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "nothing at all")
	assert.Equal(t, []string{"write docs small.txt", "write docs big.txt", "write docs two.txt"}, f.files.seen())
	for _, name := range []string{"big.txt", "two.txt"} {
		_, err := f.files.get("docs", name)
		require.ErrorIs(t, err, fs.ErrNotExist, "Files kept nothing of %s", name)
	}
}

func TestFileErrors(t *testing.T) {
	cs, f := serve(t)
	c, ctx := cs.files, t.Context()
	for _, tc := range []struct {
		err  error
		code connect.Code
		msg  string
	}{
		{fs.ErrNotExist, connect.CodeNotFound, "stat a/b.txt: file does not exist"},
		{&fs.PathError{Op: "openat", Path: "/srv/stores/docs/a/b.txt", Err: syscall.EACCES}, connect.CodePermissionDenied, "stat a/b.txt: permission denied"},
		{syscall.ENOTDIR, connect.CodeFailedPrecondition, "stat a/b.txt: not a directory"},
		{syscall.ENAMETOOLONG, connect.CodeInvalidArgument, "stat a/b.txt: file name too long"},
		{errors.New("PROPFIND https://user:pw@dav.example/docs failed"), connect.CodeInternal, "stat a/b.txt: internal error"},
		{&net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}, connect.CodeInternal, "stat a/b.txt: internal error"},
		{&fs.PathError{Op: "read", Path: "/srv/stores/docs/a/b.txt", Err: syscall.EIO}, connect.CodeInternal, "stat a/b.txt: internal error"},
		{connect.NewError(connect.CodeUnavailable, errors.New("the store is busy")), connect.CodeUnavailable, "the store is busy"},
	} {
		f.files.fail(tc.err)
		_, err := c.Stat(ctx, connect.NewRequest(&runnerv1.StatRequest{Store: "docs", Name: "a/b.txt"}))
		assert.Equal(t, tc.code, connect.CodeOf(err), tc.msg)
		var ce *connect.Error
		require.ErrorAs(t, err, &ce)
		assert.Equal(t, tc.msg, ce.Message())
	}

	f.files.fail(syscall.ENOTEMPTY)
	err := rename(ctx, c, "a", "b")
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	require.ErrorContains(t, err, "rename a to b: directory not empty")
	f.files.fail(fs.ErrExist)
	assert.Equal(t, connect.CodeAlreadyExists, connect.CodeOf(mkdir(ctx, c, "f")), "Mkdir where a file stands")
}

func TestFilesNeverReadOrReplaceAFolder(t *testing.T) {
	cs, f := serve(t)
	c, ctx := cs.files, t.Context()
	require.NoError(t, mkdir(ctx, c, "dir"))
	require.NoError(t, putFile(ctx, c, "f", "x"))
	_, err := getFile(t, c, "dir")
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err), "a folder is not read as a file")
	err = rename(ctx, c, "f", "dir")
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	require.ErrorContains(t, err, `"dir" is a folder`)
	assert.NotContains(t, f.files.seen(), "read docs dir")
	assert.NotContains(t, f.files.seen(), "rename docs f dir")
}
