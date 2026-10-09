# Quick start: create and run an app

Install `aicoded`, create an app with one page, run it on your computer as a test persona, and
change the page. You need Go and Chrome; there is no Node.js or npm to install.

## What you need

- Go 1.26 or later, with `go` on `PATH`. `aicoded` from v0.3.0 on, and every app, need Go 1.26:
  with the default `GOTOOLCHAIN=auto` an older go command downloads it, and with
  `GOTOOLCHAIN=local` it refuses.
- Chrome. Firefox should work but is not tested. Safari does not work: it drops the persona
  cookie on plain http, and macOS does not resolve `*.localhost`.
- A MySQL 8 server on your computer, once the app keeps data in its SQL database.

## Install aicoded

In a checkout of the framework, run:

```sh
make install
```

It installs `aicoded` into Go's bin folder. An app this `aicoded` creates uses the checkout
through a `replace` line in its `go.mod`, which `aicoded check` accepts only from this `aicoded`
(E-LINT-011). The delivery pipeline refuses a `replace` line (E-GATE-001), so require a released
framework version before you publish.

## Create an app

```sh
aicoded init hello
```

`aicoded init` creates the folder `hello` with:

- `go.mod`, which requires the framework;
- `aicoded.yaml`, the permission list, with `app: hello`;
- `main.go`;
- `pages/index.html`, the app's one page;
- `AGENTS.md`, the rules for AI assistants that build the app.

It then runs `aicoded generate`, which writes the page's code and a `pages/dataprovider.go` for you
to fill, and `go mod tidy`. The name is 2 to 63 lowercase letters, digits and dashes, starting with
a letter and ending with a letter or digit, and no folder there may have it (E-CLI-002).

The page is plain HTML with two framework tags: `<ssr:access>` says who may open it, and
`<ssr:assets/>` writes the page's script and style tags.

```html
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<ssr:access role="*"/>
<title>hello</title>
<ssr:assets/>
</head>
<body>
<h1>hello</h1>
</body>
</html>
```

`main.go` hands the generated pages handler to `app.Main`, which connects the app to its runner:

```go
package main

import (
	"aicoded.dev/framework/app"

	"hello/pages"
)

func main() {
	app.Main(app.Options{Handler: pages.NewHandler()})
}
```

## Run it

```sh
cd hello
aicoded dev
```

`aicoded dev` is the app's runner on your computer. It prints two links:

```text
aicoded dev: the dev UI is at http://localhost:8080/_aicoded/login?token=<64 hex characters>
aicoded dev: hello at http://hello.localhost:8080/ (switch persona at http://hello.localhost:8080/_aicoded/persona)
```

- Open `http://hello.localhost:8080/` to see the app. On your computer, test personas stand in
  for the company login. Until `dev.yaml` names personas, there are two: `all-roles`, the
  default, has every role the app's pages ask for, and `no-roles` has none. Switch at
  `/_aicoded/persona`.
- The first link logs your browser in to the dev UI, where you start and stop apps and read
  their logs, traces and caught mail.

## Change the page

Greet the viewer. In `pages/index.html`, declare a value with its Go type and show it:

```html
<body>
<ssr:var name="name" type="string"/>
<h1>Hello, {{ name }}</h1>
</body>
```

Save it. `aicoded dev` generates the page again, so its `RouteData` gets a field `Name`. Fill it
in `Data` in `pages/dataprovider.go`, the file `aicoded generate` wrote for you:

```go
// Data greets the viewer.
func (p *DP) Data(ctx context.Context, _ *web.Request, _ web.ResponseWriter, data *RouteData) error {
	data.Name = auth.Viewer(ctx).Name
	return nil
}
```

Add `"aicoded.dev/framework/auth"` to the imports and save. `aicoded dev` builds and
restarts the app, and the page greets the persona.

## Check it

```sh
aicoded check
```

`aicoded check` generates, builds, vets, lints and tests every app in the folder, and lists
every problem with its code, `file:line`, fix and docs link:

```text
ok  hello
aicoded check: ok
```

Templates compile to Go, so a mistake in one is reported at its line, such as
`hello: index.html:10: E-CHK-002: invalid operation: …`. `aicoded explain E-CHK-002` prints the
page of a code.

## See also

- [Project structure](project-structure.md): the files of an app and who writes them.
- [Template syntax](template-syntax.md): the tags and attributes of a page.
- [Access](access.md): who may open a page.
- [aicoded dev](dev.md): `dev.yaml`, personas, the dev UI and running an app by hand.
- [The people example](../../examples/people): a staff directory with forms, live values, a
  database, files, mail and a call to another app.
- [Start a new app](../tasks/new-app.md): `aicoded init`, the permission list, `deps` and
  `aicoded dev`.
