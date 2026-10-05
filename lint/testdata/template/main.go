// Command template is an app whose page code, generated from its template, breaks rules.
package main

import (
	"aicoded.dev/framework/app"

	"template/pages"
)

func main() {
	app.Main(app.Options{Handler: pages.NewHandler()})
}
