package config

import (
	"context"
	"fmt"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/runner"
)

// String returns the value of setting name.
func String(ctx context.Context, name string) (string, error) {
	s := runner.From(ctx)
	if s == nil {
		return "", runner.ErrNoRunner
	}
	v, ok := s.Settings[name]
	if !ok {
		return "", errs.New("E-MAN-001", fmt.Sprintf("setting %q is not declared", name),
			fmt.Sprintf("add %q to settings in aicoded.yaml", name))
	}
	return v, nil
}
