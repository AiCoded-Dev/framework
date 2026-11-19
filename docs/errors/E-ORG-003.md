# E-ORG-003: confirmation is not the slug

The platform's operator asked `admin orgs delete` with a confirmation, typed at the prompt or given
with `--confirm`, that is not the organisation's slug. Deleting an organisation erases its apps,
publishes, change records and people at once and cannot be undone, so the platform deletes only
when the confirmation is exactly the slug. Nothing changed.

```
admin orgs delete <slug> --confirm <slug>
```

**Fix:** run the command again and confirm with the organisation's slug exactly as `admin orgs list` shows it
