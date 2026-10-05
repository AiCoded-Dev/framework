# E-FILE-001: invalid file name

File names in a store are slash-separated paths inside the store, such as `invoices/2026/42.pdf`: no leading slash, no `.` or `..` parts, no empty parts, and no part starting with `.aicoded-tmp-`, which the runner uses for files being written. The store itself (`.`) cannot be written, removed or renamed.

**Fix:** use a relative path inside the store.
