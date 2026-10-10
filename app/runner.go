package app

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"path/filepath"
	"runtime/debug"

	"connectrpc.com/connect"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
	"aicoded.dev/framework/runnerproto/socket"
)

const minCSRFKey = 32

// conn is the app's connection to its runner.
type conn struct {
	lifecycle runnerv1connect.LifecycleServiceClient
	telemetry runnerv1connect.TelemetryServiceClient
	session   *runner.Session
	keys      []ed25519.PublicKey
	// shareSocket is set when the runner reaches app.sock as another user.
	shareSocket bool
}

// connectRunner performs the handshake with the runner whose sockets are in dir.
func connectRunner(ctx context.Context, dir string) (*conn, error) {
	if dir == "" {
		return nil, errs.New("E-RUN-001", runnerproto.EnvRunnerDir+" is not set, so there is no runner to talk to",
			"start the app with aicoded dev, or run it by hand with the command aicoded dev --manual <app> prints")
	}
	hc := socket.Client(filepath.Join(dir, runnerproto.RunnerSocket))
	opts := []connect.ClientOption{connect.WithGRPC(), connect.WithReadMaxBytes(runnerproto.MaxMessageBytes)}
	c := &conn{
		lifecycle: runnerv1connect.NewLifecycleServiceClient(hc, socket.BaseURL, opts...),
		telemetry: runnerv1connect.NewTelemetryServiceClient(hc, socket.BaseURL, opts...),
	}
	resp, err := c.lifecycle.Hello(ctx, connect.NewRequest(&runnerv1.HelloRequest{
		ProtocolVersion:  runnerproto.ProtocolVersion,
		FrameworkVersion: frameworkVersion(),
	}))
	if err != nil {
		return nil, fmt.Errorf("app: runner handshake: %w", err)
	}
	h := resp.Msg
	for _, k := range h.GetViewerKeys() {
		if len(k) == ed25519.PublicKeySize {
			c.keys = append(c.keys, ed25519.PublicKey(k))
		}
	}
	if h.GetApp() == "" || len(h.GetCsrfKey()) < minCSRFKey || len(c.keys) == 0 || len(c.keys) != len(h.GetViewerKeys()) {
		return nil, errs.New("E-RUN-003", "the runner's handshake lacks the app name, the CSRF key or valid viewer keys",
			fmt.Sprintf("use a runner that speaks runner protocol %d (update aicoded)", runnerproto.ProtocolVersion))
	}
	c.session = &runner.Session{
		App:      h.GetApp(),
		Env:      h.GetEnv(),
		Version:  h.GetVersion(),
		Settings: h.GetSettings(),
		CSRFKey:  h.GetCsrfKey(),
		Dir:      dir,
		Secrets:  runnerv1connect.NewSecretsServiceClient(hc, socket.BaseURL, opts...),
		Files:    runnerv1connect.NewFilesServiceClient(hc, socket.BaseURL, opts...),
		Mail:     runnerv1connect.NewMailServiceClient(hc, socket.BaseURL, opts...),
		Rpc:      runnerv1connect.NewRpcServiceClient(hc, socket.BaseURL, opts...),
	}
	c.shareSocket = h.GetAppSocketOpen()
	return c, nil
}

func frameworkVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, d := range bi.Deps {
			if d.Path == "aicoded.dev/framework" {
				return d.Version
			}
		}
	}
	return "(devel)"
}
