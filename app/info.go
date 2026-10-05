package app

import (
	"context"

	"aicoded.dev/framework/internal/runner"
)

// Name returns the app's name as the runner knows it, or "" outside the app.
func Name(ctx context.Context) string {
	if s := runner.From(ctx); s != nil {
		return s.App
	}
	return ""
}

// Env returns the environment the app runs in (for example "dev"), or "" outside the app.
func Env(ctx context.Context) string {
	if s := runner.From(ctx); s != nil {
		return s.Env
	}
	return ""
}

// Version returns the version of the running release, or "" outside the app.
func Version(ctx context.Context) string {
	if s := runner.From(ctx); s != nil {
		return s.Version
	}
	return ""
}
