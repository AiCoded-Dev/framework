package main

import (
	"aicoded.dev/framework/app"

	"shop/deps"
	"shop/pages"
)

func main() {
	d := &deps.Deps{}
	app.Main(app.Options{Handler: pages.NewHandler(d), OnStart: d.Start})
}
