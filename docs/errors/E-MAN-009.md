# E-MAN-009: invalid email section

The `email` section of the permission list (`aicoded.yaml`) needs `from`, the one plain lower-case address the app sends from, and `to_domains`, the lower-case domains it may send to. Display names, wildcards and non-ASCII domains are not accepted.

**Fix:** write `from: name@your.domain` and `to_domains: [your.domain]`.
