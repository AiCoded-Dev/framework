# E-MAN-008: invalid data classes

Every data source in the permission list (`aicoded.yaml`) says which classes of data it holds, such as `internal`. Later, the app types that security defines will set which classes an app may hold, and a new class will need approval. This entry has no classes, or a class that is not a lower-case name.

**Fix:** add `classes: [internal]` (or the classes that apply) to the data source.
