# E-ORG-004: organisation busy

The platform's operator asked `admin orgs delete` while a release, a preview's start or end, or
another deletion of the organisation was under way. The deletion waited 10 seconds for it to end,
and it did not. Nothing changed: the organisation stays as it was.

**Fix:** run the same command again once the release, the preview change or the other deletion ends
