// Package control is the control socket of aicoded dev: JSON over HTTP/1.1 on a Unix socket in
// the workspace's private state folder, through which aicoded mcp and a second aicoded dev
// reach the running one.
package control

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/internal/errs"
)

// Protocol is the version of the control protocol. Every request and answer carries it in the
// Aicoded-Control header.
const Protocol = "1"

const (
	header      = "Aicoded-Control"
	socketName  = "control.sock"
	pingTimeout = 2 * time.Second
	// maxBody bounds a request body.
	maxBody = 1 << 20
	// maxReply bounds an answer the client reads.
	maxReply = 64 << 20
	// maxQuote bounds the part of a header value an error message repeats.
	maxQuote = 32
	// maxPath bounds the socket path; Unix socket paths may be little longer.
	maxPath = 100
)

// SocketPath returns the path of the control socket in the state folder state.
func SocketPath(state string) string { return filepath.Join(state, socketName) }

// Listen serves the control socket at path. It fails with E-DEV-012 when an aicoded dev answers
// there, or something listens there without answering within 2 s, and with E-DEV-013 when an
// aicoded dev of another version answers. It removes a socket that nothing listens on, refuses
// any other file at path with E-DEV-016 and a path too long for a socket with E-DEV-017, and
// gives the socket mode 0600.
func Listen(ctx context.Context, path string) (net.Listener, error) {
	if len(path) > maxPath {
		return nil, errs.New("E-DEV-017", "the control socket path "+path+" is too long for a Unix socket",
			"use a home folder with a shorter path")
	}
	info, err := os.Lstat(path)
	switch {
	case err == nil && info.Mode().Type() != fs.ModeSocket:
		return nil, errs.New("E-DEV-016", path+" is not a socket",
			"remove "+path+" if nothing else uses it, then start aicoded dev again")
	case err == nil:
		if err := stale(ctx, path); err != nil {
			return nil, err
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}
	var lc net.ListenConfig
	l, err := lc.Listen(ctx, "unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = l.Close()
		return nil, err
	}
	return l, nil
}

// stale returns nil when nothing listens on the socket at path, and otherwise why it must stay.
func stale(ctx context.Context, path string) error {
	pctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	st, err := Dial(path).Status(pctx)
	switch {
	case err == nil:
		return AlreadyRunning(st)
	case ctx.Err() != nil:
		return ctx.Err()
	case errors.Is(err, syscall.ECONNREFUSED), errors.Is(err, fs.ErrNotExist):
		return nil
	case errs.Code(err) != "":
		return err
	}
	return errs.New("E-DEV-012", fmt.Sprintf("an aicoded dev may already run for this workspace: its control socket %s did not answer within 2s (%v)", path, err),
		"stop the running aicoded dev, or remove "+path+" when none runs")
}

// AlreadyRunning is the E-DEV-012 error for the aicoded dev whose status is st.
func AlreadyRunning(st devapi.Status) error {
	return errs.New("E-DEV-012", fmt.Sprintf("aicoded dev already runs for %s at %s", st.Root, st.Gateway),
		fmt.Sprintf("use the running one at %s, or stop it first", st.Gateway))
}

// mismatch is the E-DEV-013 error for a peer, who, that speaks the protocol got.
func mismatch(who, got string) error {
	if got == "" {
		got = "none"
	} else {
		got = quote(got)
	}
	return errs.New("E-DEV-013", fmt.Sprintf("%s speaks control protocol %s, and this aicoded speaks %s", who, got, Protocol),
		"stop the running aicoded dev and start it again with this aicoded")
}

// quote quotes s, cut to maxQuote bytes.
func quote(s string) string {
	if len(s) > maxQuote {
		s = s[:maxQuote] + "…"
	}
	return fmt.Sprintf("%q", s)
}
