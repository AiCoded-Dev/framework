// Package flows: the containers of the standard library.
package flows

import (
	"container/list"
	"log/slog"
	"sync"
	"sync/atomic"

	"aicoded.dev/framework/web"
)

// Containers carries the URL parameter login through a sync.Map, an atomic.Value and a list.
func Containers(r *web.Request) {
	login := r.URLParam("login")
	var m sync.Map
	m.Store("login", login)
	v, _ := m.Load("login")
	slog.Info("sync.Map", "v", v) // leak: a URL parameter reaches slog.Info
	var av atomic.Value
	av.Store(login)
	slog.Info("atomic.Value", "v", av.Load()) // leak: a URL parameter reaches slog.Info
	l := list.New()
	l.PushBack(login)
	slog.Info("list", "v", l.Front().Value) // leak: a URL parameter reaches slog.Info
}
