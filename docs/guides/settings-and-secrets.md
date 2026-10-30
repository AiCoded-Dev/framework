# Settings and secrets

Settings are named values the app is configured with, such as the address of a team; secrets are
values nobody may see, such as an API key. The permission list names both, the runner holds their
values, and the app reads them with `config.String` and `secrets.Get`.

## Declaring them

```yaml
settings: [team_address, report_day]
secrets: [partner_api_key]
```

Names are lowercase letters, digits and `_`, start with a letter and appear once (E-MAN-005).
The values are never in the permission list or the repository.

## Reading a setting

```go
team, err := config.String(ctx, "team_address")
```

- A name the permission list does not declare fails with E-MAN-001.
- The runner gives the app its settings when it starts, so read them in `OnStart` and keep them
  in `deps.Deps`, as the people example does with `team_address`. A changed value takes effect
  at the app's next start.

## Reading a secret

```go
key, err := secrets.Get(ctx, "partner_api_key")
if err != nil {
	return err
}
mac := hmac.New(sha256.New, []byte(key.Reveal()))
```

- `secrets.Get` returns a `secrets.Value`. Printed, logged with `slog`, or encoded as JSON or
  text, it shows `[REDACTED]`; only `Reveal()` returns the secret. Pass that straight to where it
  is needed and never keep, log or return it: `aicoded check` refuses it in a log line, a print
  or a span (E-LINT-008).
- A name the permission list does not declare fails with E-MAN-002.

Both need the context of the app: a request's, a hook's or the one `OnStart` gets. A context
made with `context.Background()` carries no runner and fails with E-RUN-004.

## Local values

On your computer, the values come from `dev.yaml`, outside the repository, under the app's name
in its workspace:

```yaml
workspaces:
  /absolute/path/to/apps:
    apps:
      people:
        settings: {team_address: team@acme.example}
        secrets: {partner_api_key: local-test-key}
```

- An app that declares a setting or secret with no value there does not start, and its address
  shows E-DEV-003 with the missing names.
- `dev.yaml` is read again at every start of an app: after adding a value, change a file of the
  app, press Restart on the dev UI, or call the `preview` tool.
- Use test values, never production secrets.
- `aicoded dev` hides every value of an app's secrets that is 4 bytes or longer as
  `[secret <name>]` in the log lines, spans and mail it keeps or prints, and warns about a
  shorter one. The dev UI shows no setting values.

See [aicoded dev](dev.md) for the rest of `dev.yaml`.

## On the platform

On the platform, the values are set in the console, for each app, by the app's owner or a
security admin; neither the app, its builder nor an AI assistant can set them, and the console
never shows a secret's value once it is set. A release whose permission list declares a setting
or a secret with no value is refused with E-REL-022, which lists the names: ask the app's owner or
a security admin to set them, and a *Pre-approved* publish goes live by itself about a minute
later. Each change of a value is in a change record: a setting's with its value, a secret's
without it.

- The runner gives the app its settings when it starts, as `aicoded dev` does: a changed setting
  takes effect at the app's next start. A setting deleted while a release runs reaches the app as
  an empty string at its next start; the next release, or a rollback, is refused with E-REL-022
  until it has a value again.
- A secret is read from the platform's secrets store at each `secrets.Get`, so a changed secret
  takes effect at the next read, with no new release. A secret whose value is gone fails with
  E-RUN-011, and a secret the platform cannot reach for a moment fails with an error to retry
  later.
- Read a secret once, when the app starts or first needs it, and keep the value in memory: an app
  that reads its secrets more than 20 times a second, after 100 at once, or 8 at once, fails with
  E-RUN-012.
- An app's preview has values of its own, set the same way for the preview, and never reads the
  production values. A preview whose permission list declares a value with none set for the
  preview does not start: E-REL-030 lists the names. See [publishing](publishing.md).

## See also

- [The permission list](permission-list.md): the `settings` and `secrets` sections.
- [people/deps/deps.go](../../examples/people/deps/deps.go): a setting read in `Start`.
