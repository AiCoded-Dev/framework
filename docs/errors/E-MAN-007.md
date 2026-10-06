# E-MAN-007: invalid data source

An entry under `data` in the permission list (`aicoded.yaml`) names a source the platform does not know, or names one source twice. A source is `sqldb` (the app's own SQL database), `filestore:<name>` (one of its file stores) or `connector:<name>` (a company system read with each user's own access); names are lower-case letters, digits and dashes.

**Fix:** use `sqldb`, `filestore:<name>` or `connector:<name>` with a lower-case name, once each.
