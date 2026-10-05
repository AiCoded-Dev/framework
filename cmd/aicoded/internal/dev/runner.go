// Package dev is the local runner behind aicoded dev: it builds an app, serves the runner
// protocol to it, and fronts it with a gateway that stands in for the company login.
package dev

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"sync"
	"sync/atomic"

	"connectrpc.com/connect"

	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/cmd/aicoded/internal/manifest"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
)

// runnerService serves the runner protocol to one app. Each app gets its own socket, so the
// service knows its caller without trusting anything in the request.
type runnerService struct {
	manifest   manifest.Manifest
	app        string
	secrets    []string
	values     devconfig.AppValues
	env        devconfig.Env
	settings   map[string]string
	csrfKey    []byte
	viewerKeys [][]byte
	ready      chan struct{}
	once       sync.Once
	store      *store
	dropped    atomic.Uint64
	files      *fileService
	mail       *mailService
	router     *Router
	byHand     bool // the developer runs the app by hand; every Ready is printed
}

func newRunnerService(m manifest.Manifest, values devconfig.AppValues, env devconfig.Env, csrfKey []byte, viewerKeys [][]byte, st *store) *runnerService {
	settings := make(map[string]string, len(m.Settings))
	for _, n := range m.Settings {
		settings[n] = values.Settings[n]
	}
	return &runnerService{
		manifest:   m,
		app:        m.App,
		secrets:    m.Secrets,
		values:     values,
		env:        env,
		settings:   settings,
		csrfKey:    csrfKey,
		viewerKeys: viewerKeys,
		ready:      make(chan struct{}),
		store:      st,
	}
}

func (s *runnerService) handler() http.Handler {
	limit := connect.WithReadMaxBytes(runnerproto.MaxMessageBytes)
	mux := http.NewServeMux()
	mux.Handle(runnerv1connect.NewLifecycleServiceHandler(s, limit))
	mux.Handle(runnerv1connect.NewSecretsServiceHandler(s, limit))
	mux.Handle(runnerv1connect.NewTelemetryServiceHandler(s, limit))
	mux.Handle(runnerv1connect.NewRpcServiceHandler(s, limit))
	if s.files != nil {
		mux.Handle(runnerv1connect.NewFilesServiceHandler(s.files, limit))
	}
	if s.mail != nil {
		mux.Handle(runnerv1connect.NewMailServiceHandler(s.mail, limit))
	}
	return mux
}

func (s *runnerService) Hello(_ context.Context, req *connect.Request[runnerv1.HelloRequest]) (*connect.Response[runnerv1.HelloResponse], error) {
	if v := req.Msg.GetProtocolVersion(); v != runnerproto.ProtocolVersion {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errs.New("E-RUN-002",
			fmt.Sprintf("the app speaks runner protocol %d, this aicoded speaks %d", v, runnerproto.ProtocolVersion),
			"build the app with the framework version that matches aicoded"))
	}
	return connect.NewResponse(&runnerv1.HelloResponse{
		App:        s.app,
		Env:        string(s.env),
		Version:    "dev",
		CsrfKey:    s.csrfKey,
		ViewerKeys: s.viewerKeys,
		Settings:   s.settings,
	}), nil
}

func (s *runnerService) Ready(context.Context, *connect.Request[runnerv1.ReadyRequest]) (*connect.Response[runnerv1.ReadyResponse], error) {
	s.once.Do(func() { close(s.ready) })
	if s.byHand {
		fmt.Fprintf(s.store.out, "aicoded dev: %s is ready (run by hand)\n", s.app)
	}
	return connect.NewResponse(&runnerv1.ReadyResponse{}), nil
}

func (s *runnerService) GetSecret(_ context.Context, req *connect.Request[runnerv1.GetSecretRequest]) (*connect.Response[runnerv1.GetSecretResponse], error) {
	name := req.Msg.GetName()
	if !slices.Contains(s.secrets, name) {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("secret %q is not declared", name))
	}
	return connect.NewResponse(&runnerv1.GetSecretResponse{Value: []byte(s.values.Secrets[name])}), nil
}

func (s *runnerService) Export(_ context.Context, req *connect.Request[runnerv1.ExportRequest]) (*connect.Response[runnerv1.ExportResponse], error) {
	s.store.addSpans(req.Msg.GetSpans())
	s.dropped.Add(req.Msg.GetDropped())
	return connect.NewResponse(&runnerv1.ExportResponse{}), nil
}

// Call routes a call of the app to another app of the workspace.
func (s *runnerService) Call(ctx context.Context, req *connect.Request[runnerv1.CallRequest]) (*connect.Response[runnerv1.CallResponse], error) {
	if s.router == nil {
		return nil, notRunning(req.Msg.GetApp())
	}
	resp, err := s.router.Call(ctx, s.manifest, req.Msg, req.Header().Get("traceparent"))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}
