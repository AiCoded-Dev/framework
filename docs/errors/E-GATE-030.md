# E-GATE-030: a page let in a viewer without the role its access rule names

The simulated attacks (L7), which the delivery pipeline runs on the app it built, sent requests as a viewer who holds no role to every page whose access rules name a role: to load the page, to post each of its forms and to open its live connection. The access rules on a page's path admit only a viewer with a role from each, so every such request must get 403 before any code of the app runs. A page answered otherwise, so a viewer that its rules keep out could open it or use its forms. The problem is at the page's template, at line 1, and the message names the request, what the page answered and what was expected. The handlers that `aicoded generate` writes check the access rule of every template on the path, root first, before anything else the page does, and `aicoded check` lets an app serve its pages only through them (E-LINT-004), so look for what serves the page in their place. A page whose only rule is `role="*"` admits every viewer the company login lets into the app, and is not tested. See [access](../guides/access.md) and [simulated attacks](../guides/simulated-attacks.md).

```text
  trips: pages/trips/index.html:1: E-GATE-030: POST /trips (form add) as a viewer with no role: answered 303, expected 403
    fix: serve every page through the handlers aicoded generate writes, which check the access rule first
    docs: https://aicoded.dev/docs/errors/E-GATE-030
```

**Fix:** serve every page through the handlers aicoded generate writes, which check the access rule first.
