# E-DATA-002: a new data class

This is a note: it never stops a publish. The commit's permission list, `aicoded.yaml`, gives a source in `data:` a class of data that the app's permission list did not have before, such as `classes: [internal, personal]` where it had `classes: [internal]`. The app may now reach data of that class, so the delivery pipeline reports it, and the change record lists it for the people who approve what the app does.

```yaml
data:
  - source: sqldb
    classes: [internal, personal]   # personal is new
```

**Fix:** nothing, when the app needs data of this class; otherwise remove the class from data: in aicoded.yaml
