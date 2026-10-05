package sources

import (
	"context"
	"log/slog"

	"aicoded.dev/framework/config"
	"aicoded.dev/framework/secrets"
)

// Secret records a secret, which only Reveal returns, and a setting, which is not one.
func Secret(ctx context.Context) error {
	key, err := secrets.Get(ctx, "partner_api_key")
	if err != nil {
		return err
	}
	slog.InfoContext(ctx, "key", "key", key)          // clean
	slog.InfoContext(ctx, "key", "key", key.Reveal()) // leak: a secret reaches slog.InfoContext
	team, err := config.String(ctx, "team_address")
	if err != nil {
		return err
	}
	slog.InfoContext(ctx, "team", "address", team) // clean
	return nil
}
