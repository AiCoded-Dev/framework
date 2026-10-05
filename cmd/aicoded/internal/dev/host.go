package dev

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aicoded.dev/framework/cmd/aicoded/internal/control"
	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/cmd/aicoded/internal/workspace"
	"aicoded.dev/framework/internal/errs"
)

// pingTimeout bounds the wait for an answer from an aicoded dev that may be running.
const pingTimeout = 2 * time.Second

// Options are what the developer chose for Host and Run.
type Options struct {
	// Manual names the apps the developer runs by hand, as aicoded dev --manual does.
	Manual []string
	// Login prints the login link of the dev UI, as aicoded dev does. Without it, as in aicoded
	// mcp, the line names only the dev UI's address.
	Login bool
}

// Run serves every app under dir on the local runner until ctx is done, as Host does, and
// prints the same lines. It refuses a dir with no app before it reads dev.yaml.
func Run(ctx context.Context, dir string, out io.Writer, opts Options) error {
	w, err := Host(ctx, dir, out, opts)
	if err != nil {
		return unlessStopped(ctx, err)
	}
	<-ctx.Done()
	w.Close()
	return nil
}

// Host starts aicoded dev for dir with the settings in dev.yaml, watching, and serves its
// control socket. Close on the workspace also closes the socket. It refuses a dir with no app
// before it reads dev.yaml, and, with E-DEV-012, a dir that an aicoded dev runs for already,
// directly or through a folder above it, or that holds a folder an aicoded dev runs for. It
// claims the control socket before it starts any app, so that two starts never both build.
// dev.yaml is read again at every start of an app, for its values.
func Host(ctx context.Context, dir string, out io.Writer, opts Options) (*Workspace, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if _, err := workspace.Apps(root); err != nil {
		return nil, err
	}
	if sock, ok := running(ctx, root); ok {
		sctx, cancel := context.WithTimeout(ctx, pingTimeout)
		defer cancel()
		st, err := control.Dial(sock).Status(sctx)
		if err != nil {
			return nil, err
		}
		return nil, control.AlreadyRunning(st)
	}
	cfg, err := configPath()
	if err != nil {
		return nil, err
	}
	ws, err := devconfig.Load(cfg, root)
	if err != nil {
		return nil, err
	}
	state, err := stateDir(root)
	if err != nil {
		return nil, err
	}
	sock := control.SocketPath(state)
	l, err := control.Listen(ctx, sock)
	if err != nil {
		return nil, err
	}
	w, err := Open(ctx, Config{Root: root, State: state, Dev: ws, Out: out, Watch: true, Manual: opts.Manual, Login: opts.Login,
		Values: func(app string) (devconfig.AppValues, error) {
			ws, err := devconfig.Load(cfg, root)
			if err != nil {
				return devconfig.AppValues{}, err
			}
			return ws.Apps[app], nil
		}})
	if err != nil {
		_ = l.Close()
		return nil, err
	}
	sctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := control.Serve(sctx, l, w); err != nil {
			fmt.Fprintf(w.out, "aicoded dev: the control socket stopped: %v\n", err)
		}
	}()
	w.control = sync.OnceFunc(func() {
		cancel()
		<-done
		_ = os.Remove(sock)
	})
	return w, nil
}

// FindRunning returns the root and control socket of an aicoded dev that runs for dir or a
// folder above it. One that answers with another control protocol counts as running.
func FindRunning(ctx context.Context, dir string) (root, socket string, ok bool) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", "", false
	}
	for {
		if state, err := StatePath(dir); err == nil {
			sock := control.SocketPath(state)
			if info, err := os.Lstat(sock); err == nil && info.Mode().Type() == fs.ModeSocket && answers(ctx, sock) {
				return dir, sock, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", false
		}
		dir = parent
	}
}

// running returns the control socket of an aicoded dev that runs for root, for a folder above
// it, or for a folder below it. The last it finds by asking every control socket under
// ~/.local/state/aicoded but root's own for the folder it runs for.
func running(ctx context.Context, root string) (string, bool) {
	if _, sock, ok := FindRunning(ctx, root); ok {
		return sock, true
	}
	own, err := StatePath(root)
	if err != nil {
		return "", false
	}
	states := filepath.Dir(own)
	entries, err := os.ReadDir(states)
	if err != nil {
		return "", false
	}
	below := strings.TrimSuffix(root, string(filepath.Separator)) + string(filepath.Separator)
	for _, e := range entries {
		if e.Name() == filepath.Base(own) {
			continue
		}
		sock := control.SocketPath(filepath.Join(states, e.Name()))
		if info, err := os.Lstat(sock); err != nil || info.Mode().Type() != fs.ModeSocket {
			continue
		}
		sctx, cancel := context.WithTimeout(ctx, pingTimeout)
		st, err := control.Dial(sock).Status(sctx)
		cancel()
		if err == nil && strings.HasPrefix(st.Root, below) {
			return sock, true
		}
	}
	return "", false
}

// RunningLogin returns the login link of the dev UI of the aicoded dev that Host refuses dir
// for, with E-DEV-012. It reads the token from that aicoded dev's state folder, and reports false
// when none runs or its token cannot be read.
func RunningLogin(ctx context.Context, dir string) (string, bool) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	sock, ok := running(ctx, root)
	if !ok {
		return "", false
	}
	sctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	st, err := control.Dial(sock).Status(sctx)
	if err != nil {
		return "", false
	}
	token, err := readToken(filepath.Dir(sock), false)
	if err != nil {
		return "", false
	}
	return loginLink(st.Gateway, token), true
}

// answers reports whether an aicoded dev answers on the control socket sock.
func answers(ctx context.Context, sock string) bool {
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	err := control.Dial(sock).Ping(ctx)
	return err == nil || errs.Code(err) == "E-DEV-013"
}
