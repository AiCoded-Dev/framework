# E-GEN-035: image not found

An `<img src>` names a file next to the template, or `/assets/<file>` under the app's `assets/`
folder, and that file does not exist, is not a file, or lies outside the app's `pages/` and
`assets/` folders, for example through `../` or a symbolic link. The generator copies each
such image into the app with a content hash in its name, so it must be there when the app is
generated. Only images and fonts are copied: `.gif`, `.ico`, `.jpeg`, `.jpg`, `.png`, `.svg`,
`.webp`, `.woff` and `.woff2`, so a source file is never published by mistake. The copy is
embedded in the app binary, so its folder and file names may use only letters, digits, spaces
and `!#$%&()+,-.=@[]^_{}~`, may not end with a dot, and may not be names such as `CON`, `NUL`
or `.git`. Sources that start with a scheme such as `https:` or `data:`, with `//`, or with
`/` other than `/assets/`, and sources computed with `{{ }}`, are left as they are.

**Fix:** put the file next to the template, or under `assets/` and write `/assets/<file>`
