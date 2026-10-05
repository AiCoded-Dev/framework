package secrets

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"connectrpc.com/connect"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/runnerproto/runnerv1"
)

const redacted = "[REDACTED]"

// Value is a secret. It holds a pointer to the text so that printing a struct with a Value in an unexported field shows an address.
type Value struct{ p *string }

// Reveal returns the secret. Pass the result straight to where it is needed; never log it.
func (v Value) Reveal() string {
	if v.p == nil {
		return ""
	}
	return *v.p
}

func (Value) String() string { return redacted }

func (Value) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, redacted) }

func (Value) LogValue() slog.Value { return slog.StringValue(redacted) }

func (Value) MarshalJSON() ([]byte, error) { return []byte(`"` + redacted + `"`), nil }

func (Value) MarshalText() ([]byte, error) { return []byte(redacted), nil }

// Get returns secret name.
func Get(ctx context.Context, name string) (Value, error) {
	s := runner.From(ctx)
	if s == nil || s.Secrets == nil {
		return Value{}, runner.ErrNoRunner
	}
	resp, err := s.Secrets.GetSecret(ctx, connect.NewRequest(&runnerv1.GetSecretRequest{Name: name}))
	if connect.CodeOf(err) == connect.CodeNotFound {
		return Value{}, errs.New("E-MAN-002", fmt.Sprintf("secret %q is not declared", name),
			fmt.Sprintf("add %q to secrets in aicoded.yaml", name))
	}
	if err != nil {
		return Value{}, fmt.Errorf("secrets: get %q: %w", name, err)
	}
	val := string(resp.Msg.GetValue())
	return Value{p: &val}, nil
}
