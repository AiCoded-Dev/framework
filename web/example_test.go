package web_test

import (
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"aicoded.dev/framework/web"
)

// A template writes a web.SafeHTML as it is with {{$ greeting }}. Build one from markup in the
// source code and from text escaped for it, so that what a viewer typed never becomes markup.
func ExampleJoinHTML() {
	name := `Ann <script>alert(1)</script>` // as a viewer typed it
	greeting := web.JoinHTML(web.HTMLConst("<strong>Hello</strong>, "), web.EscapeHTML(name))
	fmt.Println(greeting)
	// Output: <strong>Hello</strong>, Ann &lt;script&gt;alert(1)&lt;/script&gt;
}

// A hook returns web.Error to show the viewer a message. In Validate<Name>, which checks a value
// the page writes, the message reaches the page's ssr.onError.
func ExampleError() {
	validateDisplayName := func(name string) (string, error) {
		if utf8.RuneCountInString(name) > 50 {
			return "", web.Error(http.StatusUnprocessableEntity, "Use at most 50 characters.")
		}
		return name, nil
	}
	_, err := validateDisplayName(strings.Repeat("a", 51))
	fmt.Println(err)
	// Output: 422 Use at most 50 characters.
}

// A Process hook sends the viewer on after a post with return web.Redirect("/notes/42").
// Redirect goes only to a path on this site and refuses any other target.
func ExampleRedirect() {
	err := web.Redirect("https://elsewhere.example/")
	fmt.Println(err)
	// Output:
	// E-WEB-002: web.Redirect got a target that is not a path on this site
	//   fix: redirect to a path that starts with a single "/", such as "/notes/42"
	//   docs: https://aicoded.dev/docs/errors/E-WEB-002
}
