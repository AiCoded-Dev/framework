# E-FILE-003: a folder too large to list

`ReadDir` lists one folder of a store: its files and the folders directly in it. The runner lists
at most a bounded folder, and the bound depends on where the app runs: 10 000 entries In your own
cloud, and a listing of at most 4 MiB Hosted by us, which holds fewer entries the longer their
names are. This folder holds more, so the runner lists nothing rather than part of it. Where the
runner lists a folder to remove it, it refuses to remove such a folder with this code too, and
names the removal in the message. Reading, writing and removing the files the app knows by name
still work.

**Fix:** keep fewer files and folders directly in one folder: spread them over folders below it, such as one for each year or each first letter of their names; to remove such a folder, remove what it holds first.
