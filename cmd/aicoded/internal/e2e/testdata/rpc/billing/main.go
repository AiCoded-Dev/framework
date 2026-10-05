package main

import (
	"aicoded.dev/framework/app"

	"billing/rpc"
)

func main() {
	app.Main(app.Options{RPC: rpc.Server()})
}
