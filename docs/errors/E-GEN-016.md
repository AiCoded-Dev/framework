# E-GEN-016: reserved folder name

A folder under `pages/` has a name the framework uses itself. A page's live connection opens at
`<page>/__ws`, and aicoded generate writes the built scripts, styles and images to
`pages/assets_gen/`. The runner owns `/_aicoded/` on the app's host: the framework serves the
app's built files under `/_aicoded/assets/`, `aicoded dev` its persona switch at
`/_aicoded/persona`, and on the platform the runner serves the company login at
`/_aicoded/login`, `/_aicoded/callback` and `/_aicoded/logout`. These names cannot be page
folders at any depth, and aicoded generate does not look inside such a folder.

**Fix:** rename the folder; `__ws`, `_aicoded` and `assets_gen` are reserved
