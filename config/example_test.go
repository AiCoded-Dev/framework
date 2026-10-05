package config_test

import (
	"context"

	"aicoded.dev/framework/config"
	"aicoded.dev/framework/mailer"
)

// Read a setting where it is used. The permission list declares it with settings: [team_address],
// and its local value lives in dev.yaml.
func ExampleString() {
	process := func(ctx context.Context) error {
		team, err := config.String(ctx, "team_address")
		if err != nil {
			return err
		}
		_, err = mailer.Send(ctx, mailer.Message{
			IdempotencyKey: "weekly-report-2026-40",
			To:             []mailer.Address{{Address: team}},
			Subject:        "Weekly report",
			Text:           "The weekly report is ready in the app.",
		})
		return err
	}
	_ = process
}
