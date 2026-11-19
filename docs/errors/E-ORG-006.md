# E-ORG-006: no such organisation

The platform's operator asked `admin orgs delete` for a slug that no organisation has: it was
mistyped, or the organisation was deleted already. Nothing changed.

**Fix:** check the slug with `admin orgs list`
