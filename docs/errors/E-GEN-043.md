# E-GEN-043: folder name is not a Go name

A folder under `pages/` has a name that Go code or Go tools cannot use. Every folder on a page's
path becomes Go code: the page's folder is its package name, and the pages handler imports each
page under a name made of its folders. Such a folder must be ASCII letters, digits and `_`,
start with a letter, and not be `main`, `testdata` or a Go keyword such as `type`, `map` or
`import`. A folder that starts with `s_` or `n_` is a parameter folder and needs a name after
the prefix, as in `n_id`. No folder under `pages/` may start with `_` or `.` or be named
`testdata`, whether or not it holds a page: Go tools skip such folders, so the security checks
would never see the code in them. The folder name is also the page's URL segment.

**Fix:** name the folder with ASCII letters, digits and `_`, starting with a letter; `main`,
`testdata` and Go keywords such as `type` cannot be used
