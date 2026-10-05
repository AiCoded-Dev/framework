package rpc

import (
	"context"
	"errors"
	"fmt"
)

// GetIn names an invoice.
type GetIn struct {
	ID int64
}

// GetOut is an invoice.
type GetOut struct {
	Number string
	Lines  []Line
}

// Line is one line of an invoice.
type Line struct {
	Amount float64
}

// PingIn and PingOut carry nothing.
type (
	PingIn  struct{}
	PingOut struct{}
)

// Get returns the invoice in.ID.
//
//ssr:access caller=shop role=editor|admin
func Get(ctx context.Context, in GetIn) (GetOut, error) {
	if in.ID <= 0 {
		return GetOut{}, errors.New("no invoice")
	}
	return GetOut{Number: fmt.Sprintf("INV-%d", in.ID), Lines: []Line{{Amount: 9.5}}}, nil
}

// Ping answers every app that may call it.
//
//ssr:access caller=reports,shop apps=true
func Ping(ctx context.Context, in PingIn) (PingOut, error) {
	return PingOut{}, nil
}
