// Package rpc holds the functions billing serves to other apps.
package rpc

import (
	"context"
	"errors"
	"time"

	"aicoded.dev/framework/auth"
	frameworkrpc "aicoded.dev/framework/rpc"
)

// InvoiceID names an invoice.
type InvoiceID struct {
	ID int64
}

// Invoice is one invoice, with the viewer billing served it to.
type Invoice struct {
	ID       int64
	Customer string
	Lines    []Line
	Due      time.Time
	Paid     *bool
	Viewer   string
}

// Line is one line of an invoice.
type Line struct {
	Item   string
	Amount int64
}

// PingIn asks for nothing.
type PingIn struct{}

// PingOut says who called.
type PingOut struct {
	Caller string
	Viewer string
}

// GetInvoice returns invoice 1 to a viewer with the editor role.
//
//ssr:access caller=shop role=editor
func GetInvoice(ctx context.Context, in InvoiceID) (Invoice, error) {
	if in.ID != 1 {
		return Invoice{}, frameworkrpc.Errorf(frameworkrpc.NotFound, "no invoice %d", in.ID)
	}
	paid := false
	return Invoice{
		ID:       1,
		Customer: "Hotel Lisboa",
		Lines:    []Line{{Item: "Towels", Amount: 1200}, {Item: "Soap", Amount: 300}},
		Due:      time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC),
		Paid:     &paid,
		Viewer:   auth.Viewer(ctx).Subject,
	}, nil
}

// Ping says who called. Apps may call it on their own.
//
//ssr:access caller=shop apps=true
func Ping(ctx context.Context, in PingIn) (PingOut, error) {
	return PingOut{Caller: frameworkrpc.Caller(ctx), Viewer: auth.Viewer(ctx).Subject}, nil
}

// Secret is for viewers with the admin role, never for an app on its own.
//
//ssr:access caller=shop role=admin
func Secret(ctx context.Context, in PingIn) (PingOut, error) {
	return PingOut{Caller: frameworkrpc.Caller(ctx)}, nil
}

// Fail fails the way a bug does: the caller gets only "internal error".
//
//ssr:access caller=shop role=*
func Fail(ctx context.Context, in InvoiceID) (Invoice, error) {
	return Invoice{}, errors.New("ledger db-7 is locked by job 4411")
}

// Missing refuses the way a function should: the caller gets the code and the message.
//
//ssr:access caller=shop role=*
func Missing(ctx context.Context, in InvoiceID) (Invoice, error) {
	return Invoice{}, frameworkrpc.Errorf(frameworkrpc.NotFound, "no invoice %d", in.ID)
}
