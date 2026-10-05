# E-GEN-047: two pages share a route key

Two pages have the same route key, the first 8 hex digits of the SHA-256 of their path. The key
tells a page's forms and live values apart from those of the other pages on its path, so every
page needs its own. Two paths share a key only by rare chance.

**Fix:** rename one of the two folders
