package template

import (
	"fmt"

	"aicoded.dev/framework/internal/errs"
)

// errAt returns a coded template error at file:line. It panics when code has no fix line in
// fixes or fix is empty, because that is a bug in the generator.
func errAt(file string, line int, code, fix, format string, args ...any) *errs.Error {
	if fixes[code] == "" || fix == "" {
		panic("template: no fix line for " + code)
	}
	return errs.At(fmt.Sprintf("%s:%d", file, line), code, fmt.Sprintf(format, args...), fix)
}

// Fix returns the fix line of a generator error code, or "" for an unknown code.
func Fix(code string) string { return fixes[code] }

// fixes holds the fix line of every E-GEN code. A code's docs page is required where the
// code is raised, so the keys are built by gen.
var fixes = map[string]string{
	gen(1):  `fix the HTML at this line: close every quote and tag, write a stray "<" as &lt;, give each attribute once, write <title></title> rather than <title/>, and put only text in <noscript> and <iframe>`,
	gen(2):  `add name="…"`,
	gen(3):  `add a Go type, for example type="string"`,
	gen(4):  "use access, var, call, content, assets, form, input, select, textarea or json",
	gen(5):  "use ssr:if, ssr:else-if, ssr:else, ssr:for or ssr:bind",
	gen(6):  "put it on the element right after the one with ssr:if or ssr:else-if",
	gen(7):  `write "item in list" or "i, item in list"`,
	gen(8):  "put <ssr:input>, <ssr:select> and <ssr:textarea> inside an <ssr:form>",
	gen(9):  "close the outer <ssr:form> before opening another",
	gen(10): "give the form a name made of letters and digits; enctype is application/x-www-form-urlencoded or multipart/form-data",
	gen(11): "remove enctype or set it to multipart/form-data",
	gen(12): "use string, bool, int, int8–int64, uint, uint8–uint64, float32 or float64",
	gen(13): "give every input with this name the same gotype",
	gen(14): "name fields with letters and digits; only checkbox and radio inputs may share a name",
	gen(15): "fix the expression inside {{ }}",
	gen(16): "rename the folder; __ws, _aicoded and assets_gen are reserved",
	gen(17): `declare the variable in this template with reactive="true" client-writable="true"`,
	gen(18): "use a native <input>, <select> or <textarea> for live binding",
	gen(19): "bind a string, bool or number, or write the value from index.ts with ssr.set()",
	gen(20): "move the code to index.ts next to the template and a frame's page into a page of the app loaded with src; pass data with <ssr:json>",
	gen(21): "move the rules to index.css next to the template",
	gen(22): "attach the handler in index.ts with addEventListener",
	gen(23): "give the element a class and style it in index.css",
	gen(24): "put the attribute value in double quotes",
	gen(25): "use a fixed name; values go only in quoted attribute values and text",
	gen(26): "move the {{ }} out of <iframe>, <noscript>, <noembed>, <noframes>, <xmp> or <plaintext>",
	gen(27): "use a fixed value for this attribute",
	gen(28): "link to a page, or attach behaviour in index.ts",
	gen(29): "use {{ }}; {{$ }} works only in element text",
	gen(30): `add <ssr:access role="…"/> to this template or to a parent folder's template`,
	gen(31): `write one <ssr:access role="a,b"/> per template; roles are lower-case names or "*"`,
	gen(32): `add guard="true" to this route's <ssr:access>, or remove the Guard method`,
	gen(33): `write <ssr:json name="lower-case-id" value="expression"/>`,
	gen(34): `show the value elsewhere, or drop reactive="true"`,
	gen(35): "put the file next to the template, or under assets/ and write /assets/<file>",
	gen(36): "fix the file at this line",
	gen(37): "run aicoded in the app's directory, next to go.mod",
	gen(38): `make the value the whole attribute, or start it with a fixed path or scheme such as "/", "?" or "https://"`,
	gen(39): "keep one <ssr:content/> per template",
	gen(40): `write <ssr:call name="lowerCamelName" in="InType" out="OutType"/> once per name`,
	gen(41): "keep one parameter folder per folder",
	gen(42): "rename it; w, s, the names of the packages generated code imports, the names the page's generated files declare, such as routeKey, state, RouteData and NewHandler, and names starting with _html or renderBlock_ are reserved, and so are r, ctx, conn and msg in a variable's type and p, r, ctx, in, name and args in a call's types",
	gen(43): "name the folder with ASCII letters, digits and _, starting with a letter; main, testdata and Go keywords such as type cannot be used",
	gen(45): "use a type name, or a pointer, slice, array, map or func of them; declare other types in a .go file next to the template",
	gen(46): `move <ssr:form>, <ssr:content/> and <ssr:assets/> out of the part that changes, or drop reactive="true"`,
	gen(47): "rename one of the two folders",
	gen(48): `show the value outside <ssr:form>, or drop reactive="true"`,
	gen(49): `name a page below this one by its folders, such as default="settings", or leave default out and implement DefaultRoute`,
	gen(50): "add <ssr:assets/> inside <head> of the top layout, such as pages/index.html, or of the page itself when no layout is above it",
	gen(51): "move <ssr:assets/> and <ssr:content/> out of ssr:if and ssr:for, so they always render",
	gen(52): "wrap the top layout, usually pages/index.html, in <!doctype html><html>…</html>; below a root gate each page writes its own document",
	gen(53): `list only the roles this page adds; when it adds none, remove this <ssr:access>, or keep one role with guard="true"`,
	gen(54): `add guard="true" to the page's <ssr:access> and check the viewer in its Guard, or shared="true" when everyone the rule admits may see every record`,
	gen(55): `keep shared="true" only on a page with an id in its URL, without guard="true" on it or above it`,
}

func gen(n int) string { return fmt.Sprintf("E-GEN-%03d", n) }
