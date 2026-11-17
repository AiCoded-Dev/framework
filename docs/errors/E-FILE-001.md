# E-FILE-001: invalid file name

File names in a store are slash-separated paths inside the store, such as `invoices/2026/42.pdf`: no leading slash, no `.` or `..` parts, no empty parts, and no part starting with `.aicoded-tmp-`, which the runner uses for files being written. A name is at most 1024 bytes, each part at most 255 bytes, with no control characters. The store itself (`.`) cannot be written, removed or renamed.

In your own cloud a store's files are objects under a folder of the cloud's object storage, whose names hold at most 1024 bytes with that folder's own name, so a file name there is shorter: about 920 bytes at the least, and the message names the store's exact limit.

**Fix:** use a relative path inside the store.
