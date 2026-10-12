# E-REL-011: no hosting for apps

The platform makes each app's database, file buckets, mailbox and gateway entry at its hosting, when
the app is first released and when a release declares one the app lacks. This control plane's
configuration names no hosting, so it can release no app. Nothing was released.

**Fix:** add the hosting section and its service account key to the control plane's configuration, as the platform's README describes, restart the control plane, then release again
