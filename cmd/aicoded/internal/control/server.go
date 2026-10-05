package control

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"time"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/internal/errs"
)

const shutdownTimeout = 5 * time.Second

type appRequest struct {
	App string `json:"app"`
}

type traceRequest struct {
	ID string `json:"id"`
}

type mailGetRequest struct {
	App string `json:"app"`
	ID  string `json:"id"`
}

type mailReceiveRequest struct {
	App  string             `json:"app"`
	Mail devapi.InboundMail `json:"mail"`
}

type idReply struct {
	ID string `json:"id"`
}

// wireError is an error as it crosses the socket.
type wireError struct {
	Code string `json:"code,omitempty"`
	Pos  string `json:"pos,omitempty"`
	Msg  string `json:"msg"`
	Fix  string `json:"fix,omitempty"`
}

type errorReply struct {
	Error wireError `json:"error"`
}

// Serve answers the requests that reach l from b until ctx is done, then waits up to 5 s for
// the requests in flight.
func Serve(ctx context.Context, l net.Listener, b devapi.Backend) error {
	srv := &http.Server{
		Handler:           handler(b),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       time.Minute,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    64 << 10,
	}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(l) }()
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		_ = srv.Close()
	}
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// handler answers the control requests from b. It refuses a request of another protocol with
// E-DEV-013.
func handler(b devapi.Backend) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/status", func(w http.ResponseWriter, r *http.Request) {
		st, err := b.Status(r.Context())
		reply(w, st, err)
	})
	route(mux, "/v1/restart", func(ctx context.Context, in appRequest) (devapi.App, error) { return b.Restart(ctx, in.App) })
	route(mux, "/v1/logs", b.Logs)
	route(mux, "/v1/traces", b.Traces)
	route(mux, "/v1/trace", func(ctx context.Context, in traceRequest) (devapi.Trace, error) { return b.Trace(ctx, in.ID) })
	route(mux, "/v1/mail", func(ctx context.Context, in appRequest) ([]devapi.MailSummary, error) { return b.Mail(ctx, in.App) })
	route(mux, "/v1/mail/get", func(ctx context.Context, in mailGetRequest) (devapi.Mail, error) {
		return b.MailGet(ctx, in.App, in.ID)
	})
	route(mux, "/v1/mail/receive", func(ctx context.Context, in mailReceiveRequest) (idReply, error) {
		id, err := b.MailReceive(ctx, in.App, in.Mail)
		return idReply{ID: id}, err
	})
	route(mux, "/v1/what-broke", func(ctx context.Context, in appRequest) (devapi.Broken, error) { return b.WhatBroke(ctx, in.App) })
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(header, Protocol)
		if got := r.Header.Get(header); got != Protocol {
			fail(w, http.StatusBadRequest, mismatch("the request", got))
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// route answers POST requests to path with the JSON of what f returns for the JSON body.
func route[In, Out any](mux *http.ServeMux, path string, f func(context.Context, In) (Out, error)) {
	mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
		var in In
		if status, err := decode(w, r, &in); err != nil {
			fail(w, status, err)
			return
		}
		out, err := f(r.Context(), in)
		reply(w, out, err)
	})
}

// decode reads one JSON value of at most maxBody bytes, with no unknown fields and nothing after
// it, into v.
func decode(w http.ResponseWriter, r *http.Request, v any) (int, error) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	err := dec.Decode(v)
	if err == nil && dec.Decode(&struct{}{}) != io.EOF {
		err = errors.New("the request body holds more than one JSON value")
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return http.StatusRequestEntityTooLarge, errors.New("the request body is larger than 1 MiB")
	}
	return http.StatusBadRequest, err
}

func reply(w http.ResponseWriter, v any, err error) {
	if err != nil {
		fail(w, http.StatusUnprocessableEntity, err)
		return
	}
	write(w, http.StatusOK, v)
}

func fail(w http.ResponseWriter, status int, err error) {
	e := wireError{Msg: err.Error()}
	var coded *errs.Error
	if errors.As(err, &coded) {
		e = wireError{Code: coded.Code, Pos: coded.Pos, Msg: coded.Msg, Fix: coded.Fix}
	}
	write(w, status, errorReply{Error: e})
}

func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
