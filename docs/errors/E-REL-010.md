# E-REL-010: release still pending

An app has at most one release waiting for its server at a time. The platform's operator asked for
a release of an app while another of its releases is still pending: the runner of its server has not
reported yet whether that release serves. The message names that release. Nothing was released.

**Fix:** wait until the server's runner reports on the pending release, which `admin releases list <org> <app>` shows, then release again
