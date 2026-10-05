package web

import (
	"context"
	"log/slog"

	"aicoded.dev/framework/internal/redact"
)

// LogError logs msg at the error level with attrs, and err as "err": its text under aicoded dev
// in environment `dev`, and only its kinds and codes anywhere else.
//
// Generated code only.
func LogError(ctx context.Context, msg string, err error, attrs ...any) {
	slog.ErrorContext(ctx, msg, append(attrs, "err", redact.For(ctx, err))...)
}
