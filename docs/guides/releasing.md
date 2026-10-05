# Releasing: before you publish

An app will be published from a commit, through the platform's delivery pipeline and its
security checks, which come later. This guide lists what the commit must hold, and the checks you
can run on your computer first.

## What the commit holds

- The app's own files: `go.mod` and `go.sum`, the permission list, `main.go`, `deps/`, the pages
  with their data providers, scripts, styles and images, and `rpc/`.
- Every generated file, current: `*_gen.go`, `*_gen.ts`, `pages/assets_gen/`, `services/`,
  `.aicoded/`, and the `access` and `services` sections of `aicoded.yaml`. The security checks
  will generate the app again and refuse it when a generated file differs (E-CHK-001), so
  generated files are never edited by hand.
- A `go.mod` that requires a released framework version, with no `replace` line, no `go.work`
  file and no `vendor` folder, and no GOFLAGS entry outside the short list that E-LINT-011
  allows. An `aicoded` installed from a checkout with `make install` writes a `replace` line to
  that checkout instead, which only that `aicoded` accepts, and the delivery pipeline will refuse
  a `replace` line.

## Checking it first

```sh
aicoded check --frozen
```

- It writes nothing, and reports every generated file that is out of date (E-CHK-001), as the
  security checks will. Run `aicoded generate`, or `aicoded check` without `--frozen`, and commit
  what it writes.
- It builds, vets, lints and tests every app, once lint's precheck of GOFLAGS and the `vendor`
  folder passes: `go build`, `go vet`, the [rules for app code](rules.md) (E-LINT codes) and
  `go test`. The delivery pipeline will run the
  tests with `-race`; `aicoded check` does when cgo works, and says so when it cannot.
- Every problem comes with its code, `file:line`, fix and docs link; `aicoded explain <code>`
  prints the page.

## What security sees

Security will approve what the app may do, not its code. The permission list is that statement, so
read it before you publish:

- `data` names everything the app stores or reads, with its classes.
- `email` names where its mail can go.
- `settings` and `secrets` name what it is configured with.
- `access`, which `aicoded generate` writes, names who may open each page, and `services` which
  apps it calls and which may call it.

A page with no access rule does not generate at all, and the framework holds the app to the rest
when it runs: see [the permission list](permission-list.md).

## What the framework does not check yet

Some rules are yours to keep, since nothing checks them yet:

- A value a viewer sends is checked, beyond its type and options, in the hook that uses it; see
  [forms](forms.md).
- A mail's idempotency key names the event, never the attempt; see [mail](mail.md).

## Running without the platform

An app runs only under a runner: on your computer that is `aicoded dev`. A minimal runner for
running an app in production without the platform, `aicoded run`, comes later.

## See also

- [Tools](tools.md): `aicoded check` and `aicoded generate`.
- [Project structure](project-structure.md): which files are yours and which are generated.
