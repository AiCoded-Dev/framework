# E-REL-012: app host not a DNS name

Each app is served at a host of its own, `<app>.<org>.apps.aicoded.cloud`, which needs a
certificate. Each part of the host is a DNS label: 1 to 63 of `a-z`, `0-9` and `-`, starting and
ending with a letter or digit. Certificate authorities also refuse a label with `--` as its third
and fourth characters, which are kept for international names. The app's name or its
organisation's slug is not such a label, so the app has no host. Nothing was released.

**Fix:** rename the app with `admin apps rename <org> <app> <name>` to a name without `--` as its third and fourth characters; the apps of an organisation whose slug ends with `-` cannot be released, so ask the platform's operator for an organisation with another slug
