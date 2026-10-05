package tools

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/web"
)

func call(t *testing.T, name, args string) (any, error) {
	c, ok := NewRoute(NewDP(nil)).NewState().(web.Caller)
	require.True(t, ok, "a page with calls is a web.Caller")
	assert.Equal(t, routeKey, c.RouteKey())
	return c.Call(t.Context(), nil, name, json.RawMessage(args))
}

func TestCallAdd(t *testing.T) {
	out, err := call(t, "add", `{"a": 2, "b": 3, "note": "x"}`)
	require.NoError(t, err)
	assert.Equal(t, AddOut{Sum: 5}, out)
}

func TestRefusedCalls(t *testing.T) {
	for _, c := range []struct {
		name, args string
		status     int
	}{
		{"add", `{"a": 1, "Secret": "x"}`, http.StatusBadRequest},
		{"add", `{"a": 1, "c": 2}`, http.StatusBadRequest},
		{"add", `{"a": 1} {}`, http.StatusBadRequest},
		{"add", `{"a": "1"}`, http.StatusBadRequest},
		{"Add", `{}`, http.StatusNotFound},
		{"sub", `{}`, http.StatusNotFound},
	} {
		out, err := call(t, c.name, c.args)
		assert.Nil(t, out, c)
		var he *web.HTTPError
		if assert.ErrorAs(t, err, &he, c) {
			assert.Equal(t, c.status, he.Status, c)
		}
	}
}
