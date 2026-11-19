# E-ORG-001: the operator's own organisation

The platform's operator asked `admin orgs delete` for the operator's own organisation: the one the
operator signs in with to run the platform. The platform never deletes it, since no one could then
sign in to run the platform. Nothing changed.

**Fix:** check the slug: the operator's own organisation cannot be deleted
