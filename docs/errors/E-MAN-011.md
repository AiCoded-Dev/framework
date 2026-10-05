# E-MAN-011: file store not declared

The app opened a file store that its permission list (`aicoded.yaml`) does not declare. Every store is declared under `data` with the classes of data it holds, so security sees where the app keeps files.

**Fix:** add `- source: filestore:<name>` with `classes: [...]` to `data` in `aicoded.yaml`.
