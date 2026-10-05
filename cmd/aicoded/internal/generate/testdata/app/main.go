package main

import (
	"aicoded.dev/framework/app"

	"example.com/app/deps"
	"example.com/app/pages"
)

func main() {
	app.Main(app.Options{Handler: pages.NewHandler(&deps.Deps{})})
}
