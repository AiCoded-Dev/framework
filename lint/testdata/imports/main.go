// Command imports is an app whose main imports only what app code may.
package main

import (
	"fmt"
	"net/http"

	"aicoded.dev/framework/app"

	"imports/render"
)

func main() {
	fmt.Println(render.Name, http.StatusOK)
	app.Main(app.Options{})
}
