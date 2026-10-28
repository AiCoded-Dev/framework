# E-GATE-040: data above the app type's class

This is a note: it does not stop the publish. It is a reason its release *Needs approval*, and `aicoded publish` lists it under `Needs approval:`. A data source of the app holds a class of data above the highest one its app type allows. The classes go from the least sensitive to the most: `public`, `internal`, `confidential` and `personal`; the Internal tool allows up to `internal`. The class that counts is the highest of the classes in `data` and the class your security admin set for the source in the source registry, so the note can come even when `data` says less. See [app types](../guides/app-types.md).

```yaml
data:
  - source: sqldb
    classes: [internal, personal]   # personal is above internal, the Internal tool's highest
```

**Fix:** an approver sees the data class as new on the approval card; to stay Pre-approved, keep the app's data within the app type's class, or ask your security admin for an app type that allows it
