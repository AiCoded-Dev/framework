package main

import (
	"aicoded.dev/framework/app"

	"site/deps"
	"site/pages"
)

func main() {
	app.Main(app.Options{Handler: pages.NewHandler(deps.New())})
}
