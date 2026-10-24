# E-PUB-017: a read of logs or traces that is not valid

`aicoded logs` and `aicoded traces`, and the `logs`, `traces` and `trace` tools of `aicoded mcp`
with `env` set to `preview` or `production`, read an app's logs and traces on the platform. The
read has a value that `aicoded` or the platform does not take, and the message names it:

- the environment is `preview` or `production`, and must be given;
- a time is in RFC 3339, such as `2026-10-24T09:00:00Z`, or, in `aicoded` and its tools, a while
  ago, such as `30m`, `6h` or `2d`; `since` must be before `until`;
- a level is `DEBUG`, `INFO`, `WARN` or `ERROR`, the lowest shown;
- a trace id is 32 of `0-9` and `a-f`, as `aicoded traces` and the logs print it;
- a limit is a whole number from 1 to 1000;
- the app is named by the `app:` line of its `aicoded.yaml`.

The platform reads nothing for such a request, and no value of it reaches a query.

**Fix:** give env preview or production, times in RFC 3339, a level of DEBUG, INFO, WARN or ERROR, and a trace id of 32 of 0-9 and a-f.
