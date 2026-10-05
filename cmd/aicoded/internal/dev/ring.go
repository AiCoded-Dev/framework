package dev

import (
	"slices"
	"sync"
)

// Ring keeps the last n values added.
type Ring[T any] struct {
	mu   sync.Mutex
	buf  []T
	next int
	full bool
}

// NewRing returns a ring that keeps n values.
func NewRing[T any](n int) *Ring[T] {
	return &Ring[T]{buf: make([]T, n)}
}

// Add keeps v, dropping the oldest value when the ring is full.
func (r *Ring[T]) Add(v T) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf[r.next] = v
	r.next = (r.next + 1) % len(r.buf)
	if r.next == 0 {
		r.full = true
	}
}

// All returns the kept values, oldest first.
func (r *Ring[T]) All() []T {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.full {
		return slices.Clone(r.buf[:r.next])
	}
	return append(slices.Clone(r.buf[r.next:]), r.buf[:r.next]...)
}
