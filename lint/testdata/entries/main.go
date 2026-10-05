// Command entries is an app that hands app.Main its generated entry points.
package main

import (
	"aicoded.dev/framework/app"

	"entries/pages"
	"entries/rpc"
)

func main() {
	app.Main(app.Options{Handler: pages.NewHandler(), RPC: rpc.Server()})
}
