package server

import (
	"context"
	"crypto/ed25519"
	"io"
	"net/http"
	"time"

	"connectrpc.com/connect"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/viewer"
)

// CodedError returns the error a provider gives the app for a problem of the error catalogue: a
// *connect.Error of c whose message is the code, such as E-MAIL-008, msg, the one-line fix and the
// code's docs link, as the framework's own coded errors print them.
func CodedError(c connect.Code, code, msg, fix string) error {
	return connect.NewError(c, errs.New(code, msg, fix))
}

// Providers do the work of the building blocks on the target the app runs on. The server calls
// them only with what passed its checks.
//
// A *connect.Error a provider returns reaches the app as it is, so its code and message must be
// meant for the app. Any other error reaches the app as an internal error, without its text,
// except the file errors that Files describes.
type Providers struct {
	// Files keeps the file stores. It must be set when the app declares one.
	Files Files
	// Mail sends the app's mail and serves its mailbox. It must be set when the app declares email.
	Mail Mail
	// Secrets holds the values of the app's secrets and settings. It must be set when the app
	// declares either.
	Secrets Secrets
	// Telemetry takes the app's spans. With nil, they are dropped.
	Telemetry Telemetry
	// Calls carries the app's calls to other apps, as Router does. With nil, every call fails
	// with E-RPC-011.
	Calls Calls
	// Outbound sends the app's requests to outside services. It must be set when the app
	// declares egress.
	Outbound Outbound
}

// FileInfo describes a file or a folder in a store.
type FileInfo struct {
	// Name is the last element of the file's name.
	Name    string
	Size    int64
	Dir     bool
	ModTime time.Time
}

// MaxFileSize bounds one file in a store, in bytes.
const MaxFileSize = 1 << 30

// Files keeps an app's file stores. The server calls it only for a store the app declares, with
// a name that fs.ValidPath accepts, of at most 1024 bytes, whose elements are at most 255 bytes,
// hold no control character and do not start with ".aicoded-tmp-", a prefix Files may use for
// files of its own. A name that Write, Mkdir, Remove or Rename changes is never ".".
//
// Its errors wrap fs.ErrNotExist, fs.ErrExist, fs.ErrPermission or a syscall.Errno, from which
// the server makes the app's error: a code and a message that names the file as the app named
// it. Any other error reaches the app as an internal error.
type Files interface {
	// Stat describes the file or folder name.
	Stat(ctx context.Context, store, name string) (FileInfo, error)
	// ReadDir lists the folder name, in any order.
	ReadDir(ctx context.Context, store, name string) ([]FileInfo, error)
	// Read opens the file name, which Stat found to be a file.
	Read(ctx context.Context, store, name string) (io.ReadCloser, error)
	// Write replaces the file name with what data yields, at most MaxFileSize bytes, whole or not
	// at all: when data or ctx fails, the store stays as it was.
	Write(ctx context.Context, store, name string, data io.Reader) (FileInfo, error)
	// Mkdir makes the folder name and its missing parents. A folder that exists is no error.
	Mkdir(ctx context.Context, store, name string) error
	// Remove removes the file or the empty folder name.
	Remove(ctx context.Context, store, name string) error
	// Rename renames oldName to newName and replaces a file at newName. The server never asks it
	// to replace a folder.
	Rename(ctx context.Context, store, oldName, newName string) error
}

// Mail sends an app's mail and serves its mailbox. The server checks every message against the
// mail rules and the app's email section first, and every folder, page and message id the app
// names. A target that keeps no mailbox answers List, Get, Delete and Move with a
// *connect.Error, E-MAIL-008 with the code Unimplemented.
type Mail interface {
	// Send sends msg from the address from, the app's email.from, once for each idempotency key
	// it remembers, and returns the message's id; a repeated key it remembers returns the first
	// message's id.
	Send(ctx context.Context, from string, msg *runnerv1.SendRequest) (string, error)
	// List returns a page of the messages in folder, newest first, and how many it holds.
	List(ctx context.Context, folder string, offset, limit int) ([]*runnerv1.MessageSummary, int, error)
	// Get returns the message id.
	Get(ctx context.Context, id string) (*runnerv1.GetResponse, error)
	// Delete deletes the message id.
	Delete(ctx context.Context, id string) error
	// Move moves the message id to folder.
	Move(ctx context.Context, id, folder string) error
}

// Secrets holds the values of an app's secrets and settings.
type Secrets interface {
	// Secret returns the value of the secret name, which the app declares.
	Secret(ctx context.Context, name string) (string, error)
	// Settings returns the values of the settings. Hello hands the app those it declares. It may
	// be called from several goroutines at once, and the server never changes the map.
	Settings() map[string]string
}

// Telemetry takes an app's spans. Each span's trace id is empty or 16 bytes, and its span and
// parent ids empty or 8 bytes. Its name and error are at most 4 KiB, and it has at most 64
// attributes, with keys of at most 256 bytes and values of at most 4 KiB. One export holds at
// most 1024 spans.
type Telemetry interface {
	// Export takes spans, and dropped, the number of spans the app or the server dropped since
	// the previous export.
	Export(ctx context.Context, spans []*runnerv1.Span, dropped uint64)
}

// Calls carries the calls of an app to other apps.
type Calls interface {
	// Call checks req, a call made by the app of caller, and forwards it with traceparent.
	Call(ctx context.Context, caller manifest.Manifest, req *runnerv1.CallRequest, traceparent string) (*runnerv1.CallResponse, error)
}

// Outbound sends an app's requests to outside services. The server hands it only a request it
// checked against egress: https to port 443 of a host egress lists, by its canonical name, with
// the key egress binds to the host in its header, no hop-by-hop or Host header and a body of at
// most runnerproto.MaxOutboundBody bytes.
//
// Do resolves and dials the host, follows no redirect and bounds the response body to
// runnerproto.MaxOutboundBody bytes and the whole request to runnerproto.OutboundDeadline,
// until the body is closed. A refusal or failure meant for the app is an *errs.Error of the
// framework's error catalogue, returned by Do or by a read of the body; it reaches the app by its
// code and message. Any other error reaches the app as E-OUT-008, without its text.
type Outbound interface {
	Do(ctx context.Context, req *http.Request) (*http.Response, error)
}

// Minter signs the tokens the runner hands apps.
type Minter interface {
	// Public returns the key that verifies the tokens it signs.
	Public() ed25519.PublicKey
	// MintCall returns the token of a call from the app caller to the app callee: on behalf of
	// the viewer v, or of the calling app itself when v is nil.
	MintCall(v *viewer.Claims, caller, callee string, now time.Time) (string, error)
}
