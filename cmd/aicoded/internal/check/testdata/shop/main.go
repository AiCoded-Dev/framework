// Command shop is a one-page app for the tests of aicoded check.
package main

import (
	"aicoded.dev/framework/app"

	"shop/pages"
)

func main() {
	app.Main(app.Options{Handler: pages.NewHandler()})
}
