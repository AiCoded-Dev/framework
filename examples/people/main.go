// People is a staff directory: staff look up their colleagues, and hr adds new ones.
package main

import (
	"aicoded.dev/framework/app"

	"people/deps"
	"people/pages"
)

func main() {
	d := deps.New()
	app.Main(app.Options{Handler: pages.NewHandler(d), OnStart: d.Start})
}
