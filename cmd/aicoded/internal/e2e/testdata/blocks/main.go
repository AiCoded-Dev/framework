package main

import (
	"aicoded.dev/framework/app"

	"blocks/deps"
	"blocks/pages"
)

func main() {
	d := &deps.Deps{}
	app.Main(app.Options{Handler: pages.NewHandler(d), OnStart: d.Start})
}
