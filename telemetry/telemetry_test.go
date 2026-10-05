package telemetry_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/telemetry"
)

func TestSpans(t *testing.T) {
	var got []*runnerv1.Span
	ctx := runner.With(context.Background(), &runner.Session{Export: func(s *runnerv1.Span) { got = append(got, s) }})

	ctx, root := telemetry.Start(ctx, "root")
	_, child := telemetry.Start(ctx, "child")
	child.SetAttr("room", "12")
	child.RecordError(errors.New("boom"))
	child.End()
	child.End()
	root.End()

	require.Len(t, got, 2)
	c, r := got[0], got[1]
	assert.Equal(t, "child", c.GetName())
	assert.Equal(t, r.GetTraceId(), c.GetTraceId())
	assert.Equal(t, r.GetSpanId(), c.GetParentSpanId())
	assert.Empty(t, r.GetParentSpanId())
	assert.Equal(t, map[string]string{"room": "12"}, c.GetAttributes())
	assert.Equal(t, "*errors.errorString", c.GetError(), "the kind of the error, not its text")
	assert.LessOrEqual(t, c.GetStartUnixNano(), c.GetEndUnixNano())
}

func TestRecordError(t *testing.T) {
	err := fmt.Errorf("add alice: %w", fs.ErrExist)
	for env, want := range map[string]string{
		"":        "fs.ErrExist",
		"preview": "fs.ErrExist",
		"dev":     "add alice: file already exists",
	} {
		var got *runnerv1.Span
		ctx := runner.With(context.Background(), &runner.Session{Env: env, Export: func(s *runnerv1.Span) { got = s }})
		_, s := telemetry.Start(ctx, "users.add")
		s.RecordError(err)
		s.End()
		assert.Equal(t, want, got.GetError(), env)
	}
}

// textless is an error whose text must not be built.
type textless struct{ t *testing.T }

func (e textless) Error() string {
	e.t.Error("the error's text was built outside aicoded dev")
	return "alice"
}

func TestRecordErrorBuildsNoTextOutsideDev(t *testing.T) {
	var got *runnerv1.Span
	ctx := runner.With(context.Background(), &runner.Session{Env: "preview", Export: func(s *runnerv1.Span) { got = s }})
	_, s := telemetry.Start(ctx, "users.add")
	s.RecordError(textless{t})
	s.End()
	assert.Equal(t, "telemetry_test.textless", got.GetError())
}

func TestSpanWithoutRunner(t *testing.T) {
	_, s := telemetry.Start(context.Background(), "orphan")
	assert.NotPanics(t, s.End)
}

func TestNoUpdatesAfterEnd(t *testing.T) {
	var got *runnerv1.Span
	ctx := runner.With(context.Background(), &runner.Session{Export: func(s *runnerv1.Span) { got = s }})

	_, s := telemetry.Start(ctx, "span")
	s.SetAttr("a", "1")
	s.End()
	s.SetAttr("b", "2")
	s.RecordError(errors.New("late"))

	require.NotNil(t, got)
	assert.Equal(t, map[string]string{"a": "1"}, got.GetAttributes())
	assert.Empty(t, got.GetError())
}
