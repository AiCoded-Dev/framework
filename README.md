# aicoded framework

Open Go framework for building company web apps with AI assistants. Pages are rendered on the
server from templates that compile to type-checked Go, and every page has an access rule.
Building blocks give an app the viewer's identity, its own SQL database, file stores, mail,
settings, secrets, telemetry and calls to other apps, with no password or key in the app. A CLI,
a local runner and a local MCP server come with it; queues, schedules, outbound HTTP and a test
kit come later.

Module: `aicoded.dev/framework`

> **Status:** pre-release. No API is stable and nothing here is ready for use yet.

## Quick start

You need Go 1.25 or later and Chrome. `make install` puts `aicoded` in `$(go env GOPATH)/bin`,
which must be on your `PATH`. In a checkout of this repository:

```sh
make install          # installs aicoded from this checkout
aicoded init hello    # creates the app hello in a new folder
cd hello
aicoded dev           # runs it on your computer
```

Open `http://hello.localhost:8080/` and switch test personas at
`http://hello.localhost:8080/_aicoded/persona`. `aicoded dev` also prints a login link to its dev
UI, where you start and stop the apps and read their logs, traces and caught mail.

The [quick start guide](docs/guides/quickstart.md) goes on from there: what the new app holds,
how to change its page and how to check it.

## Docs

The docs live in `docs/` and are built into `aicoded`: `aicoded explain <topic>` prints one
offline, and the `howto` tool of `aicoded mcp` gives the same pages to your AI assistant.
[llms.txt](llms.txt) lists them all.

- [Guides](docs/guides/overview.md), starting with the overview of how an app works:
  - pages: the quick start, project structure, template syntax, expressions, routing, access,
    forms, live values, page calls, assets, the TypeScript API, the web API and the request
    pipeline;
  - building blocks, calls and tools: the permission list, the SQL database, file stores, mail,
    settings and secrets, telemetry, calls between apps, `aicoded dev`, the tools, MCP and
    releasing.
- [Error catalogue](docs/errors/README.md): every error code, with its fix.
- [Examples](examples/README.md): people, a staff directory; contacts, the app it calls; and
  room-maintenance, a list of tickets with an ownership check.
- [Changelog for AI assistants](docs/changelog-agents.md): changes to the API, newest first.

## Requirements

Go 1.25 or later. An app that keeps data in its SQL database needs a MySQL 8 server on your
computer to run under `aicoded dev`.

## Checks

```sh
make tools   # installs golangci-lint, govulncheck, gitleaks, buf and the protobuf plugins
make check   # checks the generated protobuf code, in every module runs go test -race,
             # golangci-lint run and govulncheck, and runs make secrets
```

- Tests that need MySQL read the admin DSN of a MySQL 8 server from `AICODED_TEST_MYSQL_DSN` and
  skip without it; `AICODED_E2E_MYSQL=required` makes them fail instead. The browser tests need
  Chrome, and `AICODED_E2E_BROWSER=required` makes them fail without it.
- `make docs` rewrites `llms.txt` and the code the docs quote from the examples; `go test ./docs/`
  fails until both are current.
- `make secrets` looks for leaked passwords and keys with gitleaks and its default rules: in every
  commit reachable from `HEAD`, and in every file git tracks or would track, so other branches,
  the stash and ignored folders are not scanned. An inline `gitleaks:allow` has no effect. A
  finding is fixed at its source; one that stays in an old commit is listed by its commit
  fingerprint, `<commit>:<file>:<rule>:<line>`, in `.gitleaksignore`, with a comment saying what
  it is. `make secrets` refuses any other line in that file, and any file path with a colon.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
