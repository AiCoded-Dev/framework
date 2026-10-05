# E-DEV-020: env in dev.yaml is not dev or preview

A workspace in `dev.yaml` sets `env:` to something other than `dev` or `preview`. `aicoded dev` runs a workspace's apps in one of these two environments: `dev`, its own, or `preview`, which runs them as a preview so that the framework behaves as it does outside `aicoded dev` and records errors and panics by their kinds and codes only. The message names the line and never quotes the value.

**Fix:** set `env: dev` or `env: preview` at that line, or remove the line to run the apps as `dev`.
