package rpc_test

import (
	"errors"
	"fmt"

	"aicoded.dev/framework/rpc"
	"aicoded.dev/framework/web"
)

// A function of an app's rpc/ package tells its caller why it failed with one of six codes. Any
// other error reaches the caller as Internal, with the message "internal error".
func ExampleError() {
	err := rpc.Errorf(rpc.NotFound, "no user %q", "zoe")
	fmt.Println(rpc.CodeOf(err), err)
	fmt.Println(rpc.CodeOf(errors.New("database is locked")))
	// Output:
	// not_found no user "zoe"
	// internal
}

// The calling page turns the codes it expects into answers for the viewer, and fails on any
// other error.
func ExampleCodeOf() {
	answer := func(err error) error {
		switch rpc.CodeOf(err) {
		case 0:
			return nil
		case rpc.PermissionDenied:
			return web.Forbidden()
		case rpc.NotFound:
			return web.NotFound()
		default:
			return err
		}
	}
	fmt.Println(answer(rpc.Error(rpc.PermissionDenied, "not your contacts")))
	fmt.Println(answer(rpc.Error(rpc.NotFound, "no such user")))
	// Output:
	// 403 You do not have access to this page.
	// 404 This page does not exist.
}
