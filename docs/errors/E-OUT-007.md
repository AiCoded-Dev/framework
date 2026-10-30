# E-OUT-007: too many requests at once

Each instance of an app has at most 16 requests to outside services under way at once. The
runner refuses the 17th at once rather than queue it. Nothing was sent.

**Fix:** wait for a request to finish before starting another
