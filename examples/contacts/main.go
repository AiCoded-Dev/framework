// Contacts serves the phone numbers and email addresses of people to the people app.
package main

import (
	"aicoded.dev/framework/app"

	"contacts/deps"
	"contacts/rpc"
)

func main() {
	app.Main(app.Options{RPC: rpc.Server(), OnStart: deps.Start})
}
