# E-MAN-019: invalid schedule

`schedule` in the permission list (`aicoded.yaml`) lists the app's scheduled jobs, each with `cron` and `job`. `job` names the job: lower-case letters, digits and dashes, starting with a letter, each job once. `cron` says when it runs, in UTC, as five fields separated by spaces: minute (0-59), hour (0-23), day of the month (1-31), month (1-12) and weekday (0-6, with 0 for Sunday). A field is `*`, a number, a range `a-b`, a step `*/n` or `a-b/n`, or a comma list of them, such as `1,15`. Names such as `MON`, and `?`, `L`, `W`, `#` and `@daily`, are not accepted. The message names the field at fault.

```yaml
schedule:
  - {cron: "@daily", job: daily-summary}      # wrong: not five fields
  - {cron: "0 6 * * *", job: daily-summary}   # right: 06:00 UTC every day
```

See [the permission list](../guides/permission-list.md).

**Fix:** write each job as `{cron: "<minute> <hour> <day> <month> <weekday>", job: <name>}`, such as `{cron: "0 6 * * *", job: daily-summary}` for 06:00 UTC every day, each job once.
