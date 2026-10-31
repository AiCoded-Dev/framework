// Command clients is an app whose pages share clients of outside services.
package main

import (
	"aicoded.dev/framework/app"

	"clients/deps"
	"clients/pages"
)

func main() {
	app.Main(app.Options{Handler: pages.NewHandler(deps.New(), deps.NewExposed())})
}
