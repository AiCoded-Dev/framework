# E-WEB-006: two parameter folders in one folder

A folder under `pages/` holds two parameter folders, such as `n_id` and `s_slug`, so a URL
segment could match either of them. The pages handler refuses to start rather than pick one.
aicoded generate refuses this layout; this error means the generated code is out of date.

**Fix:** keep one parameter folder per folder and run `aicoded generate`.
