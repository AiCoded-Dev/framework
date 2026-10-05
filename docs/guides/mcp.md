# MCP: the tools for your AI assistant

`aicoded mcp` gives your AI assistant the tools of `aicoded` over MCP: it creates apps, checks
them, previews them in `aicoded dev`, reads what went wrong, and reads these docs offline. No
tool runs a shell command or takes a file path, and nothing a tool returns holds a secret value.

## Setting it up

`aicoded mcp [dir]` serves MCP on standard input and output for the workspace `dir`, the current
folder by default. For Claude Code, run this in the workspace folder:

```sh
claude mcp add aicoded -- aicoded mcp
```

The MCP setup page of the dev UI shows the commands for Claude Code and Codex, and the
`.cursor/mcp.json` for Cursor, with the full paths of this `aicoded` and of the workspace.

## The tools

| Tool | What it does |
|---|---|
| `app_create` | creates a new app in its own folder of the workspace, as `aicoded init` does, with its `AGENTS.md` |
| `check` | runs lint's precheck of GOFLAGS and the `vendor` folder, `aicoded generate`, `go build`, `go vet`, the lint rules for app code and the tests for every app or one, and lists each problem with its code, `file:line` and fix; `frozen` writes nothing |
| `preview` | builds and starts, or restarts, an app in `aicoded dev`, and returns its state and local address, or the problems that stop it |
| `what_broke` | lists what went wrong: the problems of failed apps, the last error log lines and the last failed spans |
| `describe` | summarises the apps, as `aicoded describe` does |
| `howto` | shows a page of the docs, or lists them all with their summaries |
| `logs` | the newest log lines of the apps, by app, lowest level, trace or text |
| `traces` | the newest traces, each request or call across the apps, by app or with an error only |
| `trace` | every span of one trace, in start order |
| `mail_list` | the mail an app sent, which `aicoded dev` caught, and its mailbox, newest first |
| `mail_get` | one message from `mail_list` |
| `mail_receive` | delivers a test message to an app's inbox, as if it had arrived by mail |

All but `app_create`, `check`, `preview` and `mail_receive` only read. Tools name apps by the
`app:` line of their `aicoded.yaml`, never by a folder or a path (E-DEV-014).

## The docs

`howto` serves the same pages as `aicoded explain`:

- with no topic, every page of the docs with its summary, then every error code with its title;
- with a topic, its text: a guide such as `guides/forms`, a task recipe such as
  `tasks/add-form`, `changelog`, or an error code such as `E-DEV-001`.

They are also the MCP resources `aicoded://docs/<path>` for the guides and the task recipes,
such as `aicoded://docs/guides/forms` and `aicoded://docs/tasks/add-form`,
`aicoded://errors/<CODE>` for the error pages, and `aicoded://changelog` for the
[changelog for AI assistants](../changelog-agents.md). An unknown topic is E-CLI-001.

## The loop

The server tells the assistant to start with `howto guides/overview`, and to work in this loop:

1. `app_create` makes a new app. Then it edits the app's files.
2. `check` generates the code, builds the apps and runs `go vet`, the lint rules for app code
   and the tests. It fixes every problem listed.
3. `preview` builds and starts an app in `aicoded dev` and returns its local address.
4. When something fails, `what_broke` lists the failed apps, error log lines and failed spans.

It writes `app`, `owner`, `data`, `email`, `settings` and `secrets` in the permission list by
hand, and never edits what `aicoded generate` writes: the `access` and `services` sections, every
`*_gen.go` and `*_gen.ts` file, `pages/assets_gen/` and `.aicoded/`.

## Which aicoded dev it uses

- When an `aicoded dev` runs for the folder or a folder above it, `aicoded mcp` uses it, through
  its control socket.
- Otherwise the first tool that needs one starts it inside `aicoded mcp`, printing to standard
  error, until the session ends. That one prints only the address of its dev UI; to log in, run
  `aicoded dev` in the folder, which refuses (E-DEV-012) and prints the login link.
- An `aicoded dev` of another version is refused (E-DEV-013): stop it and start it again with
  this `aicoded`.

## See also

- [Tools](tools.md): the same commands on the command line.
- [aicoded dev](dev.md): the dev UI and the workspace the tools work on.
