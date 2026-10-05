# E-GEN-016: reserved folder name

A folder under `pages/` has a name the framework uses itself. A page's live connection opens at
`<page>/__ws`, the framework serves its own files under `/_aicoded/`, and aicoded generate
writes the built scripts, styles and images to `pages/assets_gen/`. These names cannot be page
folders at any depth, and aicoded generate does not look inside such a folder.

**Fix:** rename the folder; `__ws`, `_aicoded` and `assets_gen` are reserved
