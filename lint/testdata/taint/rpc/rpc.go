// Package rpc holds the functions the app serves to other apps.
package rpc

import (
	"context"
	"log/slog"
)

// LookupIn names a person.
type LookupIn struct {
	Login string
	ID    int64
}

// LookupOut is the team of a person.
type LookupOut struct {
	Team string
}

// Lookup returns the team of a person.
//
//ssr:access caller=shop role=staff
func Lookup(ctx context.Context, in LookupIn) (LookupOut, error) {
	slog.InfoContext(ctx, "lookup", "login", in.Login) // leak: the input of an RPC function reaches slog.InfoContext
	slog.InfoContext(ctx, "lookup", "id", in.ID)       // clean
	return LookupOut{Team: "ops"}, nil
}
