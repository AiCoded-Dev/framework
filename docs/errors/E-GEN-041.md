# E-GEN-041: two parameter folders in one folder

A folder under `pages/` holds two parameter folders, such as `n_id` and `s_slug`, and both
lead to pages. A URL segment could match either of them, so aicoded generate refuses the layout
rather than pick one. A folder named `s_name` takes any one URL segment as the parameter
`name`; a folder named `n_name` takes only a decimal integer. A page that needs another
parameter goes under a fixed folder, such as `notes/n_id` and `notes/by-slug/s_slug`.

**Fix:** keep one parameter folder per folder
