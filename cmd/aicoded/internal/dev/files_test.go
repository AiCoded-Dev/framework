package dev

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
)

// filesClient serves a file service with the given stores and returns a client and the folder
// that holds the stores.
func filesClient(t *testing.T, stores ...string) (runnerv1connect.FilesServiceClient, string) {
	dir := t.TempDir()
	svc, err := newFileService(dir, stores)
	require.NoError(t, err)
	t.Cleanup(svc.close)
	mux := http.NewServeMux()
	mux.Handle(runnerv1connect.NewFilesServiceHandler(svc))
	return runnerv1connect.NewFilesServiceClient(serveSocket(t, mux), socketBaseURL, connect.WithGRPC()), dir
}

func putFile(ctx context.Context, c runnerv1connect.FilesServiceClient, name string, chunks ...string) error {
	s := c.Write(ctx)
	if err := s.Send(&runnerv1.WriteRequest{Part: &runnerv1.WriteRequest_Header{Header: &runnerv1.WriteHeader{Store: "docs", Name: name}}}); err != nil {
		_, err = s.CloseAndReceive()
		return err
	}
	for _, chunk := range chunks {
		if err := s.Send(&runnerv1.WriteRequest{Part: &runnerv1.WriteRequest_Data{Data: []byte(chunk)}}); err != nil {
			_, err = s.CloseAndReceive()
			return err
		}
	}
	_, err := s.CloseAndReceive()
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

func listDir(t *testing.T, c runnerv1connect.FilesServiceClient, name string) []string {
	resp, err := c.ReadDir(t.Context(), connect.NewRequest(&runnerv1.ReadDirRequest{Store: "docs", Name: name}))
	require.NoError(t, err)
	names := []string{}
	for _, e := range resp.Msg.GetEntries() {
		names = append(names, e.GetName())
	}
	return names
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

// storeEntries counts the entries of the store docs under dir, temporary files included.
func storeEntries(dir string) int {
	entries, err := os.ReadDir(filepath.Join(dir, "docs"))
	if err != nil {
		return -1
	}
	return len(entries)
}

func TestFileService(t *testing.T) {
	c, dir := filesClient(t, "docs")
	ctx := t.Context()
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(putFile(ctx, c, "a/b.txt", "x")), "the folder must exist")
	require.NoError(t, mkdir(ctx, c, "a/deep"), "Mkdir creates parents")
	require.NoError(t, mkdir(ctx, c, "a/deep"), "an existing folder is not an error")
	require.NoError(t, putFile(ctx, c, "a/b.txt", "hello ", "world"))
	got, err := getFile(t, c, "a/b.txt")
	require.NoError(t, err)
	assert.Equal(t, "hello world", got)
	require.NoError(t, putFile(ctx, c, "a/b.txt", "new"))
	got, _ = getFile(t, c, "a/b.txt")
	assert.Equal(t, "new", got, "a write replaces the file")

	assert.Equal(t, []string{"b.txt", "deep"}, listDir(t, c, "a"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", tmpPrefix+"planted"), nil, 0o600))
	assert.Equal(t, []string{"a"}, listDir(t, c, "."), "temporary files never show")

	require.NoError(t, rename(ctx, c, "a/b.txt", "a/c.txt"))
	_, err = c.Stat(ctx, connect.NewRequest(&runnerv1.StatRequest{Store: "docs", Name: "a/b.txt"}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	_, err = getFile(t, c, "a")
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err), "a folder is not read as a file")
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(remove(ctx, c, "a")), "a folder that is not empty stays")
	require.NoError(t, remove(ctx, c, "a/c.txt"), "a file goes")
	require.NoError(t, remove(ctx, c, "a/deep"), "an empty folder goes")
	require.NoError(t, remove(ctx, c, "a"))
	assert.Empty(t, listDir(t, c, "."))
}

func TestFileServiceSweepsTemporaryFiles(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	store := filepath.Join(dir, "docs")
	require.NoError(t, os.MkdirAll(filepath.Join(store, "a"), 0o700))
	for _, p := range []string{tmpPrefix + "x", "a/" + tmpPrefix + "y", "keep.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(store, p), []byte("x"), 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(outside, tmpPrefix+"z"), []byte("x"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(store, "link")))

	svc, err := newFileService(dir, []string{"docs"})
	require.NoError(t, err)
	t.Cleanup(svc.close)
	assert.NoFileExists(t, filepath.Join(store, tmpPrefix+"x"))
	assert.NoFileExists(t, filepath.Join(store, "a", tmpPrefix+"y"))
	assert.FileExists(t, filepath.Join(store, "keep.txt"))
	assert.FileExists(t, filepath.Join(outside, tmpPrefix+"z"), "a link out of the store is not followed")
}

func TestFileServiceConflicts(t *testing.T) {
	c, dir := filesClient(t, "docs")
	ctx := t.Context()
	require.NoError(t, mkdir(ctx, c, "full/sub"))
	require.NoError(t, mkdir(ctx, c, "empty"))
	require.NoError(t, putFile(ctx, c, "f", "x"))

	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(putFile(ctx, c, "empty", "y")), "a write onto a folder")
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(rename(ctx, c, "f", "empty")), "a file onto a folder")
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(rename(ctx, c, "empty", "full")), "a folder onto a folder that is not empty")
	err := rename(ctx, c, "full", "empty")
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err), "a folder onto an empty folder")
	require.ErrorContains(t, err, `"empty" is a folder`)
	for _, name := range []string{"f", "f/sub"} {
		assert.Equal(t, connect.CodeAlreadyExists, connect.CodeOf(mkdir(ctx, c, name)), "Mkdir where a file stands: %q", name)
	}

	assert.Equal(t, []string{"empty", "f", "full"}, listDir(t, c, "."), "the store is unchanged")
	assert.Equal(t, 3, storeEntries(dir), "no temporary file stays")
	got, err := getFile(t, c, "f")
	require.NoError(t, err)
	assert.Equal(t, "x", got)
}

func TestFileErrorsNameOnlyTheStorePath(t *testing.T) {
	c, dir := filesClient(t, "docs")
	ctx := t.Context()
	require.NoError(t, putFile(ctx, c, "f", "x"))
	_, err := c.ReadDir(ctx, connect.NewRequest(&runnerv1.ReadDirRequest{Store: "docs", Name: "f"}))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	require.ErrorContains(t, err, " f: ")
	assert.NotContains(t, err.Error(), dir)

	err = putFile(ctx, c, "nope/x.txt", "x")
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	require.ErrorContains(t, err, " nope/x.txt: ")
	assert.NotContains(t, err.Error(), tmpPrefix)
}

func TestFileServiceRefusesNames(t *testing.T) {
	c, _ := filesClient(t, "docs")
	ctx := t.Context()
	for _, name := range []string{"../x", "/etc/passwd", "a/../../x", "", "a//b", ".aicoded-tmp-x", "a/.aicoded-tmp-y", "a/.aicoded-tmp-z/b"} {
		_, err := c.Stat(ctx, connect.NewRequest(&runnerv1.StatRequest{Store: "docs", Name: name}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), name)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(putFile(ctx, c, name, "x")), name)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(mkdir(ctx, c, name)), name)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(rename(ctx, c, "x", name)), name)
	}
	for _, name := range []string{".", ""} {
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(remove(ctx, c, name)), "the store itself cannot be changed: %q", name)
	}
	for _, name := range []string{"a\x00b", strings.Repeat("a", 300)} {
		_, err := c.Stat(ctx, connect.NewRequest(&runnerv1.StatRequest{Store: "docs", Name: name}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "the system refuses %q", name)
	}
	_, err := c.Stat(ctx, connect.NewRequest(&runnerv1.StatRequest{Store: "other", Name: "."}))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err), "an undeclared store")
}

func TestFileServiceStaysInsideTheStore(t *testing.T) {
	c, dir := filesClient(t, "docs")
	outside := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "docs", "link.txt")))
	require.NoError(t, os.Symlink(filepath.Dir(outside), filepath.Join(dir, "docs", "linkdir")))

	_, err := getFile(t, c, "link.txt")
	require.Error(t, err, "a link out of the store is not followed")
	require.Error(t, putFile(t.Context(), c, "linkdir/secret.txt", "overwritten"))
	require.NoError(t, putFile(t.Context(), c, "link.txt", "replaced"), "a write replaces the link, not its target")
	data, err := os.ReadFile(outside)
	require.NoError(t, err)
	assert.Equal(t, "secret", string(data))
}

func TestFileWritesAreWholeOrAbsent(t *testing.T) {
	c, dir := filesClient(t, "docs")
	ctx, cancel := context.WithCancel(t.Context())
	s := c.Write(ctx)
	require.NoError(t, s.Send(&runnerv1.WriteRequest{Part: &runnerv1.WriteRequest_Header{Header: &runnerv1.WriteHeader{Store: "docs", Name: "half.txt"}}}))
	require.NoError(t, s.Send(&runnerv1.WriteRequest{Part: &runnerv1.WriteRequest_Data{Data: []byte("half")}}))
	require.Eventually(t, func() bool { return storeEntries(dir) == 1 }, 2*time.Second, 10*time.Millisecond, "the write goes to a temporary file")
	cancel()
	assert.Eventually(t, func() bool { return storeEntries(dir) == 0 }, 2*time.Second, 10*time.Millisecond, "neither the file nor its temporary file stays")
	_, _ = s.CloseAndReceive()
}

func TestFileWritesStopAtTheLimit(t *testing.T) {
	defer func(old int64) { maxFileSize = old }(maxFileSize)
	maxFileSize = 4
	c, dir := filesClient(t, "docs")
	require.NoError(t, putFile(t.Context(), c, "small.txt", "12", "34"))
	assert.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(putFile(t.Context(), c, "big.txt", "12", "345")))
	assert.Equal(t, 1, storeEntries(dir), "only small.txt: neither big.txt nor its temporary file stays")
}
