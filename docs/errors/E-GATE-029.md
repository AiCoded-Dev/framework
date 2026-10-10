# E-GATE-029: a page answered a request without a valid session

The simulated attacks (L7), which the delivery pipeline runs on the app it built, sent requests without a valid session: with no viewer token, with a token that had expired, and with a token made for another app. Every page, every form post, every live connection of a page and, when the app serves functions to other apps, every call to them must answer such a request with 401 before any code of the app runs, so that nobody reaches the app's data without the company login. A page answered otherwise. The problem is at the page's template, at line 1, and the message names the request, the token it carried, what the page answered and what was expected; it never holds the token itself. The handlers that `aicoded generate` writes refuse every such request, and `aicoded check` lets an app serve its pages only through them (E-LINT-004), so look for what serves the page in their place. See [simulated attacks](../guides/simulated-attacks.md).

```text
  trips: pages/trips/n_id/index.html:1: E-GATE-029: GET /trips/7 with an expired token: answered 200, expected 401
    fix: serve every page through the handlers aicoded generate writes, which refuse a request without a valid session
    docs: https://aicoded.dev/docs/errors/E-GATE-029
```

**Fix:** serve every page through the handlers aicoded generate writes, which refuse a request without a valid session.
